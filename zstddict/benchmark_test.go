package zstddict

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

// generateFileListSamples creates sample data that mimics file listing responses.
func generateFileListSamples(count int) [][]byte {
	samples := make([][]byte, count)
	paths := []string{
		"/usr/local/bin/",
		"/home/user/documents/project/",
		"/var/log/application/",
		"/etc/config/",
		"/opt/app/lib/",
		"/System/Library/Frameworks/",
		"/Applications/Xcode.app/Contents/",
		"/private/var/folders/tmp/",
	}
	files := []string{
		"main.go", "config.yaml", "README.md", "server.log", "data.json",
		"index.html", "package.json", "Makefile", "Dockerfile", "go.mod",
		"handler.go", "service.go", "model.go", "utils.go", "test.go",
	}
	modes := []string{"drwxr-xr-x", "-rw-r--r--", "-rwxr-xr-x", "-rw-------"}
	sizes := []string{"4096", "1234", "56789", "123456", "9876543"}

	for i := range samples {
		var sb strings.Builder
		for j := 0; j < 100; j++ { // 100 files per sample
			sb.WriteString(paths[(i+j)%len(paths)])
			sb.WriteString(files[(i+j)%len(files)])
			sb.WriteString(" ")
			sb.WriteString(modes[(i+j)%len(modes)])
			sb.WriteString(" ")
			sb.WriteString(sizes[(i+j)%len(sizes)])
			sb.WriteString(" 2024-01-15T10:30:00Z\n")
		}
		samples[i] = []byte(sb.String())
	}
	return samples
}

func BenchmarkCompression(b *testing.B) {
	samples := generateFileListSamples(100)
	dict, err := TrainDict(samples, nil)
	if err != nil {
		b.Fatalf("TrainDict failed: %v", err)
	}

	// Create test data of various sizes
	smallData := samples[0][:500]                                        // ~500 bytes
	mediumData := samples[0]                                              // ~5KB
	largeData := bytes.Repeat(samples[0], 10)                            // ~50KB

	compressorPlain, _ := New()
	compressorDict, _ := New(WithDictBytes(dict))

	testCases := []struct {
		name string
		data []byte
	}{
		{"small_500B", smallData},
		{"medium_5KB", mediumData},
		{"large_50KB", largeData},
	}

	for _, tc := range testCases {
		// Zstd without dict
		b.Run("zstd_"+tc.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = compressorPlain.Compress(tc.data)
			}
		})

		// Zstd with dict
		b.Run("zstd_dict_"+tc.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = compressorDict.Compress(tc.data)
			}
		})

		// Gzip for comparison
		b.Run("gzip_"+tc.name, func(b *testing.B) {
			for b.Loop() {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				_, _ = w.Write(tc.data)
				_ = w.Close()
			}
		})
	}
}

func BenchmarkDecompression(b *testing.B) {
	samples := generateFileListSamples(100)
	dict, _ := TrainDict(samples, nil)

	compressorPlain, _ := New()
	compressorDict, _ := New(WithDictBytes(dict))

	testData := samples[0]

	// Pre-compress data
	zstdPlain, _ := compressorPlain.Compress(testData)
	zstdDict, _ := compressorDict.Compress(testData)

	var gzipBuf bytes.Buffer
	gw := gzip.NewWriter(&gzipBuf)
	gw.Write(testData)
	gw.Close()
	gzipData := gzipBuf.Bytes()

	b.Run("zstd", func(b *testing.B) {
		for b.Loop() {
			_, _ = compressorPlain.Decompress(zstdPlain)
		}
	})

	b.Run("zstd_dict", func(b *testing.B) {
		for b.Loop() {
			_, _ = compressorDict.Decompress(zstdDict)
		}
	})

	b.Run("gzip", func(b *testing.B) {
		for b.Loop() {
			gr, _ := gzip.NewReader(bytes.NewReader(gzipData))
			_, _ = io.ReadAll(gr)
			gr.Close()
		}
	})
}

func TestCompressionRatios(t *testing.T) {
	samples := generateFileListSamples(100)
	dict, err := TrainDict(samples, nil)
	if err != nil {
		t.Fatalf("TrainDict failed: %v", err)
	}

	compressorPlain, _ := New()
	compressorDict, _ := New(WithDictBytes(dict))

	testCases := []struct {
		name string
		data []byte
	}{
		{"small_500B", samples[0][:500]},
		{"medium_5KB", samples[0]},
		{"large_50KB", bytes.Repeat(samples[0], 10)},
	}

	t.Logf("Dictionary size: %d bytes", len(dict))
	t.Log("")
	t.Logf("%-15s %10s %10s %10s %10s %10s %10s", "Size", "Original", "Gzip", "Gzip%", "Zstd", "Zstd%", "ZstdDict%")

	for _, tc := range testCases {
		original := len(tc.data)

		// Gzip
		var gzipBuf bytes.Buffer
		gw := gzip.NewWriter(&gzipBuf)
		gw.Write(tc.data)
		gw.Close()
		gzipSize := gzipBuf.Len()

		// Zstd plain
		zstdPlain, _ := compressorPlain.Compress(tc.data)

		// Zstd with dict
		zstdDict, _ := compressorDict.Compress(tc.data)

		t.Logf("%-15s %10d %10d %9.1f%% %10d %9.1f%% %9.1f%%",
			tc.name,
			original,
			gzipSize, float64(gzipSize)/float64(original)*100,
			len(zstdPlain), float64(len(zstdPlain))/float64(original)*100,
			float64(len(zstdDict))/float64(original)*100,
		)
	}
}
