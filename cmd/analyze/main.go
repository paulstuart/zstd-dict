// Command analyze demonstrates bandwidth savings from dictionary compression.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"os"

	"github.com/paulstuart/zstd-dict/server"
	"github.com/paulstuart/zstd-dict/zstddict"
)

func main() {
	sampleDir := flag.String("dir", "/usr/local", "Directory to sample")
	numRequests := flag.Int("n", 100, "Simulate N requests")
	realistic := flag.Bool("realistic", false, "Run realistic scenarios instead")
	flag.Parse()

	if *realistic {
		RunRealisticScenarios()
		return
	}

	fmt.Println("=== Dictionary Compression Bandwidth Analysis ===\n")

	// Generate training samples
	fmt.Printf("Generating samples from: %s\n", *sampleDir)
	samples, err := server.GenerateResponseSamples([]string{*sampleDir}, 20, 500)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating samples: %v\n", err)
		os.Exit(1)
	}

	if len(samples) < 100 {
		fmt.Fprintf(os.Stderr, "Not enough samples (got %d, need 100+)\n", len(samples))
		os.Exit(1)
	}

	// Train dictionary
	fmt.Printf("Training dictionary from %d samples...\n", len(samples))
	dict, err := zstddict.TrainDict(samples[:200], &zstddict.TrainDictOptions{
		MaxDictSize: 16 * 1024, // 16KB max
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error training dictionary: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Dictionary size: %d bytes\n\n", len(dict))

	// Create compressors
	compNone, _ := zstddict.New()
	compDict, _ := zstddict.New(zstddict.WithDictBytes(dict))

	// Simulate requests
	testSamples := samples[200:]
	if len(testSamples) > *numRequests {
		testSamples = testSamples[:*numRequests]
	}

	var (
		totalUncompressed int64
		totalGzip         int64
		totalZstd         int64
		totalZstdDict     int64
	)

	fmt.Printf("Simulating %d file listing requests...\n\n", len(testSamples))

	for i, sample := range testSamples {
		totalUncompressed += int64(len(sample))

		// Gzip
		var gzipBuf bytes.Buffer
		gw := gzip.NewWriter(&gzipBuf)
		gw.Write(sample)
		gw.Close()
		totalGzip += int64(gzipBuf.Len())

		// Zstd plain
		compressed, _ := compNone.Compress(sample)
		totalZstd += int64(len(compressed))

		// Zstd with dict
		compressedDict, _ := compDict.Compress(sample)
		totalZstdDict += int64(len(compressedDict))

		// Show progress every 20 requests
		if (i+1)%20 == 0 {
			fmt.Printf("  After %d requests:\n", i+1)
			fmt.Printf("    Uncompressed:  %8d bytes\n", totalUncompressed)
			fmt.Printf("    Gzip:          %8d bytes (%.1f%%)\n",
				totalGzip, float64(totalGzip)/float64(totalUncompressed)*100)
			fmt.Printf("    Zstd:          %8d bytes (%.1f%%)\n",
				totalZstd, float64(totalZstd)/float64(totalUncompressed)*100)
			fmt.Printf("    Zstd+Dict:     %8d bytes (%.1f%%) [+%d dict overhead]\n\n",
				totalZstdDict, float64(totalZstdDict)/float64(totalUncompressed)*100, len(dict))
		}
	}

	// Final summary
	fmt.Println("=== Final Results ===\n")
	fmt.Printf("Total requests:       %d\n", len(testSamples))
	fmt.Printf("Average message size: %d bytes\n\n", totalUncompressed/int64(len(testSamples)))

	dictCostIncluded := totalZstdDict + int64(len(dict))

	fmt.Printf("Method          Total Bytes    Ratio    vs Uncompressed    vs Gzip    Break-even\n")
	fmt.Println("------------------------------------------------------------------------------------")
	fmt.Printf("Uncompressed    %11d   100.0%%           -              -           -\n", totalUncompressed)
	fmt.Printf("Gzip            %11d    %.1f%%      %6d KB       -           -\n",
		totalGzip,
		float64(totalGzip)/float64(totalUncompressed)*100,
		(totalUncompressed-totalGzip)/1024)
	fmt.Printf("Zstd            %11d    %.1f%%      %6d KB   %5d KB        -\n",
		totalZstd,
		float64(totalZstd)/float64(totalUncompressed)*100,
		(totalUncompressed-totalZstd)/1024,
		(totalGzip-totalZstd)/1024)
	fmt.Printf("Zstd+Dict       %11d    %.1f%%      %6d KB   %5d KB    %d reqs\n",
		totalZstdDict,
		float64(totalZstdDict)/float64(totalUncompressed)*100,
		(totalUncompressed-totalZstdDict)/1024,
		(totalGzip-totalZstdDict)/1024,
		calculateBreakeven(dict, testSamples, compNone, compDict))
	fmt.Printf("(w/ dict cost)  %11d    %.1f%%      %6d KB   %5d KB\n\n",
		dictCostIncluded,
		float64(dictCostIncluded)/float64(totalUncompressed)*100,
		(totalUncompressed-dictCostIncluded)/1024,
		(totalGzip-dictCostIncluded)/1024)

	// Per-request savings
	avgSavingsVsGzip := (totalGzip - totalZstdDict) / int64(len(testSamples))
	fmt.Printf("Average savings per request vs gzip: %d bytes (%.1f%%)\n",
		avgSavingsVsGzip,
		float64(avgSavingsVsGzip)/float64(totalGzip/int64(len(testSamples)))*100)

	// Show size distribution
	fmt.Println("\n=== Message Size Distribution ===\n")
	showSizeDistribution(testSamples, compNone, compDict)
}

func calculateBreakeven(dict []byte, samples [][]byte, compNone, compDict *zstddict.Compressor) int {
	dictCost := len(dict)
	cumulativeSavings := 0

	for i, sample := range samples {
		plain, _ := compNone.Compress(sample)
		withDict, _ := compDict.Compress(sample)
		savings := len(plain) - len(withDict)
		cumulativeSavings += savings

		if cumulativeSavings >= dictCost {
			return i + 1
		}
	}
	return -1 // Never breaks even
}

func showSizeDistribution(samples [][]byte, compNone, compDict *zstddict.Compressor) {
	// Categorize by original size
	type bucket struct {
		minSize, maxSize int
		count            int
		totalOrig        int64
		totalZstd        int64
		totalDict        int64
	}

	buckets := []bucket{
		{0, 1000, 0, 0, 0, 0},
		{1000, 5000, 0, 0, 0, 0},
		{5000, 10000, 0, 0, 0, 0},
		{10000, 50000, 0, 0, 0, 0},
		{50000, 1000000, 0, 0, 0, 0},
	}

	for _, sample := range samples {
		size := len(sample)
		plain, _ := compNone.Compress(sample)
		withDict, _ := compDict.Compress(sample)

		for i := range buckets {
			if size >= buckets[i].minSize && size < buckets[i].maxSize {
				buckets[i].count++
				buckets[i].totalOrig += int64(size)
				buckets[i].totalZstd += int64(len(plain))
				buckets[i].totalDict += int64(len(withDict))
				break
			}
		}
	}

	fmt.Printf("Size Range         Count   Zstd Ratio   Dict Ratio   Improvement\n")
	fmt.Println("------------------------------------------------------------------")
	for _, b := range buckets {
		if b.count == 0 {
			continue
		}
		zstdRatio := float64(b.totalZstd) / float64(b.totalOrig) * 100
		dictRatio := float64(b.totalDict) / float64(b.totalOrig) * 100
		improvement := zstdRatio - dictRatio

		var rangeStr string
		if b.maxSize >= 1000000 {
			rangeStr = fmt.Sprintf("%5dK+", b.minSize/1024)
		} else if b.minSize >= 1000 {
			rangeStr = fmt.Sprintf("%2dK-%2dK", b.minSize/1024, b.maxSize/1024)
		} else {
			rangeStr = fmt.Sprintf("%4d-%4d", b.minSize, b.maxSize)
		}

		fmt.Printf("%-15s %7d      %5.1f%%      %5.1f%%      %+5.1f%%\n",
			rangeStr, b.count, zstdRatio, dictRatio, improvement)
	}
}
