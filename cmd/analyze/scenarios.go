package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/paulstuart/zstd-dict/zstddict"
)

// MetricsPayload represents a typical monitoring metrics payload
type MetricsPayload struct {
	Timestamp   int64             `json:"timestamp"`
	ServiceName string            `json:"service_name"`
	HostName    string            `json:"hostname"`
	Metrics     map[string]float64 `json:"metrics"`
	Tags        map[string]string `json:"tags"`
}

func RunRealisticScenarios() {
	scenarios := []struct {
		name        string
		generator   func(int) [][]byte
		description string
	}{
		{
			"Metrics/Telemetry",
			generateMetrics,
			"Time-series metrics with repetitive field names",
		},
		{
			"Small API Responses",
			generateAPIResponses,
			"JSON API responses with consistent schema",
		},
		{
			"File Listings",
			generateFileLists,
			"Directory listings with common path prefixes",
		},
	}

	for _, scenario := range scenarios {
		fmt.Printf("\n========================================\n")
		fmt.Printf("Scenario: %s\n", scenario.name)
		fmt.Printf("Description: %s\n", scenario.description)
		fmt.Printf("========================================\n\n")

		// Generate samples
		samples := scenario.generator(1000)

		// Train dictionary on first 20% of samples
		trainingSet := samples[:200]
		testSet := samples[200:500] // Use 300 for testing

		dict, err := zstddict.TrainDict(trainingSet, &zstddict.TrainDictOptions{
			MaxDictSize: 2048, // 2KB dictionary
		})
		if err != nil {
			fmt.Printf("Error training: %v\n", err)
			continue
		}

		compNone, _ := zstddict.New()
		compDict, _ := zstddict.New(zstddict.WithDictBytes(dict))

		// Analyze
		analyzeScenario(testSet, dict, compNone, compDict)
	}
}

func analyzeScenario(samples [][]byte, dict []byte, compNone, compDict *zstddict.Compressor) {
	var (
		totalOrig int64
		totalGzip int64
		totalZstd int64
		totalDict int64
	)

	for _, sample := range samples {
		totalOrig += int64(len(sample))

		// Gzip
		var gzipBuf bytes.Buffer
		gw := gzip.NewWriter(&gzipBuf)
		gw.Write(sample)
		gw.Close()
		totalGzip += int64(gzipBuf.Len())

		// Zstd
		compressed, _ := compNone.Compress(sample)
		totalZstd += int64(len(compressed))

		// Zstd+Dict
		compressedDict, _ := compDict.Compress(sample)
		totalDict += int64(len(compressedDict))
	}

	avgSize := totalOrig / int64(len(samples))
	dictCostIncluded := totalDict + int64(len(dict))

	fmt.Printf("Messages analyzed:    %d\n", len(samples))
	fmt.Printf("Avg message size:     %d bytes\n", avgSize)
	fmt.Printf("Dictionary size:      %d bytes\n\n", len(dict))

	fmt.Printf("Compression Results:\n")
	fmt.Printf("  Uncompressed:  %7d KB  (100.0%%)\n", totalOrig/1024)
	fmt.Printf("  Gzip:          %7d KB   (%.1f%%)\n",
		totalGzip/1024, float64(totalGzip)/float64(totalOrig)*100)
	fmt.Printf("  Zstd:          %7d KB   (%.1f%%)\n",
		totalZstd/1024, float64(totalZstd)/float64(totalOrig)*100)
	fmt.Printf("  Zstd+Dict:     %7d KB   (%.1f%%)  ← %d bytes better/msg\n\n",
		totalDict/1024, float64(totalDict)/float64(totalOrig)*100,
		(totalZstd-totalDict)/int64(len(samples)))

	// Break-even calculation
	breakeven := calculateBreakevenPoint(samples, dict, compNone, compDict)

	fmt.Printf("Break-even Analysis:\n")
	fmt.Printf("  Dict overhead:     %d bytes\n", len(dict))
	fmt.Printf("  Avg savings/msg:   %d bytes\n", (totalZstd-totalDict)/int64(len(samples)))
	fmt.Printf("  Break-even point:  %d messages\n", breakeven)
	fmt.Printf("  After %d msgs:     %.1f KB saved vs zstd\n\n",
		len(samples), float64(totalZstd-dictCostIncluded)/1024)

	// Bandwidth savings for different request volumes
	fmt.Printf("Cumulative Bandwidth Savings (vs Zstd):\n")
	volumes := []int{100, 1000, 10000, 100000}
	for _, vol := range volumes {
		if vol > len(samples)*10 {
			vol = len(samples)
		}
		factor := vol / len(samples)
		if factor == 0 {
			factor = 1
		}
		projected := (totalZstd - totalDict) * int64(factor)
		withDictCost := projected - int64(len(dict))

		fmt.Printf("  %6d messages:  %6.1f KB saved", vol, float64(withDictCost)/1024)
		if withDictCost > 0 {
			pct := float64(withDictCost) / float64(totalZstd*int64(factor)) * 100
			fmt.Printf("  (%.1f%% reduction)", pct)
		}
		fmt.Println()
	}
}

func calculateBreakevenPoint(samples [][]byte, dict []byte, compNone, compDict *zstddict.Compressor) int {
	dictCost := len(dict)
	savings := 0

	for i, sample := range samples {
		plain, _ := compNone.Compress(sample)
		withDict, _ := compDict.Compress(sample)
		savings += len(plain) - len(withDict)

		if savings >= dictCost {
			return i + 1
		}
	}
	return -1
}

func generateMetrics(count int) [][]byte {
	samples := make([][]byte, count)
	services := []string{"api-gateway", "user-service", "payment-service", "notification-service"}
	hosts := []string{"prod-node-01", "prod-node-02", "prod-node-03", "prod-node-04"}

	metricNames := []string{
		"http.requests.count", "http.requests.duration_ms",
		"db.queries.count", "db.queries.duration_ms",
		"cache.hits", "cache.misses",
		"cpu.usage_percent", "memory.usage_bytes",
		"goroutines.count", "heap.alloc_bytes",
	}

	for i := range samples {
		payload := MetricsPayload{
			Timestamp:   time.Now().Unix(),
			ServiceName: services[rand.Intn(len(services))],
			HostName:    hosts[rand.Intn(len(hosts))],
			Metrics:     make(map[string]float64),
			Tags: map[string]string{
				"environment": "production",
				"region":      "us-west-2",
				"version":     "v1.2.3",
			},
		}

		for _, name := range metricNames {
			payload.Metrics[name] = rand.Float64() * 1000
		}

		data, _ := json.Marshal(payload)
		samples[i] = data
	}
	return samples
}

func generateAPIResponses(count int) [][]byte {
	samples := make([][]byte, count)

	type User struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		CreatedAt string `json:"created_at"`
		Status    string `json:"status"`
	}

	names := []string{"john", "jane", "bob", "alice", "charlie", "diana"}
	statuses := []string{"active", "inactive", "pending"}

	for i := range samples {
		user := User{
			ID:        rand.Intn(10000),
			Username:  names[rand.Intn(len(names))] + fmt.Sprintf("%d", rand.Intn(100)),
			Email:     fmt.Sprintf("user%d@example.com", rand.Intn(1000)),
			FirstName: names[rand.Intn(len(names))],
			LastName:  names[rand.Intn(len(names))],
			CreatedAt: time.Now().Format(time.RFC3339),
			Status:    statuses[rand.Intn(len(statuses))],
		}
		data, _ := json.Marshal(user)
		samples[i] = data
	}
	return samples
}

func generateFileLists(count int) [][]byte {
	samples := make([][]byte, count)

	prefixes := []string{
		"/usr/local/lib/",
		"/var/log/application/",
		"/opt/service/config/",
		"/home/user/documents/",
	}

	files := []string{
		"config.yaml", "main.go", "handler.go", "model.go",
		"service.log", "error.log", "access.log",
		"data.json", "cache.db", "README.md",
	}

	for i := range samples {
		var buf bytes.Buffer
		numFiles := 10 + rand.Intn(20)
		for j := 0; j < numFiles; j++ {
			prefix := prefixes[rand.Intn(len(prefixes))]
			file := files[rand.Intn(len(files))]
			fmt.Fprintf(&buf, "%s%s %d bytes\n", prefix, file, rand.Intn(100000))
		}
		samples[i] = buf.Bytes()
	}
	return samples
}
