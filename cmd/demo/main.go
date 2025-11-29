// Command demo provides a CLI for demonstrating zstd dictionary compression with gRPC.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/paulstuart/zstd-dict/client"
	"github.com/paulstuart/zstd-dict/grpccodec"
	"github.com/paulstuart/zstd-dict/server"
	"github.com/paulstuart/zstd-dict/zstddict"
	pb "github.com/paulstuart/zstd-dict/proto/filelist"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "server":
		runServer(args)
	case "client":
		runClient(args)
	case "train":
		runTrain(args)
	case "bench":
		runBench(args)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: demo <command> [options]

Commands:
  server    Start the gRPC server
  client    Query the server for directory listing
  train     Generate a dictionary from sample data
  bench     Run compression benchmarks

Run 'demo <command> -h' for command-specific options.`)
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":50051", "Server address")
	dictPath := fs.String("dict", "", "Path to dictionary file (optional)")
	fs.Parse(args)

	// Register compressors
	if *dictPath != "" {
		dict, err := os.ReadFile(*dictPath)
		if err != nil {
			log.Fatalf("Failed to load dictionary: %v", err)
		}
		grpccodec.Register(dict)
		log.Printf("Loaded dictionary: %s (%d bytes)", *dictPath, len(dict))
	} else {
		grpccodec.Register(nil)
	}

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterFileListServiceServer(s, server.New())

	log.Printf("Server listening on %s", *addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func runClient(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Server address")
	path := fs.String("path", ".", "Directory to list")
	depth := fs.Int("depth", 0, "Max recursion depth (0 = unlimited)")
	compressor := fs.String("compress", "", "Compressor: zstd, zstd-dict, gzip, or empty for none")
	dictPath := fs.String("dict", "", "Path to dictionary file (for zstd-dict)")
	fs.Parse(args)

	// Register compressors if using zstd
	if *compressor == "zstd" || *compressor == "zstd-dict" {
		var dict []byte
		if *dictPath != "" {
			var err error
			dict, err = os.ReadFile(*dictPath)
			if err != nil {
				log.Fatalf("Failed to load dictionary: %v", err)
			}
		}
		grpccodec.Register(dict)
	}

	c, err := client.New(client.Options{
		Address:    *addr,
		Compressor: *compressor,
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, stats, err := c.ListFilesWithStats(ctx, *path, int32(*depth))
	if err != nil {
		log.Fatalf("ListFiles failed: %v", err)
	}

	fmt.Printf("Root: %s\n", resp.Root)
	fmt.Printf("Files: %d\n", resp.TotalCount)
	fmt.Printf("Duration: %v\n", stats.Duration)
	fmt.Println()

	// Print first 20 files
	limit := 20
	if len(resp.Files) < limit {
		limit = len(resp.Files)
	}
	for i := 0; i < limit; i++ {
		f := resp.Files[i]
		if f.IsDir {
			fmt.Printf("  [DIR]  %s\n", f.Path)
		} else {
			fmt.Printf("  %6d %s\n", f.Size, f.Path)
		}
	}
	if len(resp.Files) > limit {
		fmt.Printf("  ... and %d more\n", len(resp.Files)-limit)
	}
}

func runTrain(args []string) {
	fs := flag.NewFlagSet("train", flag.ExitOnError)
	output := fs.String("o", "filelist.dict", "Output dictionary file")
	maxSize := fs.Int("size", 32*1024, "Maximum dictionary size in bytes")
	fs.Parse(args)

	dirs := fs.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}

	log.Printf("Generating training samples from: %v", dirs)

	// Generate individual file samples (better for dictionary training)
	samples, err := server.GenerateSamples(dirs, 5000)
	if err != nil {
		log.Fatalf("Failed to generate samples: %v", err)
	}

	// Also add some response-level samples
	respSamples, _ := server.GenerateResponseSamples(dirs, 20, 100)
	samples = append(samples, respSamples...)

	log.Printf("Generated %d samples", len(samples))

	if len(samples) < 10 {
		log.Fatalf("Not enough samples for training (need at least 10, got %d)", len(samples))
	}

	dict, err := zstddict.TrainDict(samples, &zstddict.TrainDictOptions{
		MaxDictSize: *maxSize,
	})
	if err != nil {
		log.Fatalf("Failed to train dictionary: %v", err)
	}

	if err := os.WriteFile(*output, dict, 0644); err != nil {
		log.Fatalf("Failed to write dictionary: %v", err)
	}

	log.Printf("Dictionary written to %s (%d bytes)", *output, len(dict))
}

func runBench(args []string) {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Server address")
	path := fs.String("path", ".", "Directory to list")
	depth := fs.Int("depth", 0, "Max recursion depth")
	dictPath := fs.String("dict", "", "Path to dictionary file")
	iterations := fs.Int("n", 10, "Number of iterations per compressor")
	fs.Parse(args)

	// Load dictionary if provided
	var dict []byte
	if *dictPath != "" {
		var err error
		dict, err = os.ReadFile(*dictPath)
		if err != nil {
			log.Fatalf("Failed to load dictionary: %v", err)
		}
	}

	// Register all compressors
	grpccodec.Register(dict)
	_ = gzip.Name // Ensure gzip is registered

	compressors := []string{"", "gzip", "zstd"}
	if dict != nil {
		compressors = append(compressors, "zstd-dict")
	}

	fmt.Printf("Benchmarking %d iterations for path: %s\n\n", *iterations, *path)
	fmt.Printf("%-12s %10s %10s %10s\n", "Compressor", "Avg(ms)", "Min(ms)", "Max(ms)")
	fmt.Println(string(make([]byte, 50)))

	for _, comp := range compressors {
		name := comp
		if name == "" {
			name = "none"
		}

		c, err := client.New(client.Options{
			Address:    *addr,
			Compressor: comp,
		})
		if err != nil {
			log.Printf("%-12s failed to connect: %v", name, err)
			continue
		}

		var total, minD, maxD time.Duration
		minD = time.Hour

		for i := 0; i < *iterations; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, stats, err := c.ListFilesWithStats(ctx, *path, int32(*depth))
			cancel()

			if err != nil {
				log.Printf("%-12s iteration %d failed: %v", name, i, err)
				continue
			}

			total += stats.Duration
			if stats.Duration < minD {
				minD = stats.Duration
			}
			if stats.Duration > maxD {
				maxD = stats.Duration
			}
		}

		c.Close()

		avg := total / time.Duration(*iterations)
		fmt.Printf("%-12s %10.2f %10.2f %10.2f\n",
			name,
			float64(avg.Microseconds())/1000,
			float64(minD.Microseconds())/1000,
			float64(maxD.Microseconds())/1000,
		)
	}
}
