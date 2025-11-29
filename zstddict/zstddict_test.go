package zstddict

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressor_RoundTrip(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("hello world")},
		{"medium", bytes.Repeat([]byte("the quick brown fox "), 100)},
		{"large", bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz"), 10000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := c.Compress(tc.data)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			decompressed, err := c.Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress() error = %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("round trip failed: got %d bytes, want %d bytes", len(decompressed), len(tc.data))
			}
		})
	}
}

func TestCompressor_WithDict(t *testing.T) {
	// Create a temporary dictionary file
	dictPath := filepath.Join(t.TempDir(), "test.dict")

	// Train a simple dictionary from sample data
	samples := generateSampleData(100)
	dict, err := TrainDict(samples, nil)
	if err != nil {
		t.Fatalf("TrainDict() error = %v", err)
	}

	if err := os.WriteFile(dictPath, dict, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test with dict bytes
	t.Run("WithDictBytes", func(t *testing.T) {
		c, err := New(WithDictBytes(dict))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		if !c.HasDict() {
			t.Error("HasDict() = false, want true")
		}

		testData := []byte(strings.Repeat("/usr/local/bin/program ", 50))
		compressed, err := c.Compress(testData)
		if err != nil {
			t.Fatalf("Compress() error = %v", err)
		}

		decompressed, err := c.Decompress(compressed)
		if err != nil {
			t.Fatalf("Decompress() error = %v", err)
		}

		if !bytes.Equal(decompressed, testData) {
			t.Error("round trip with dict failed")
		}
	})

	// Test with dict file
	t.Run("WithDictFile", func(t *testing.T) {
		c, err := New(WithDictFile(dictPath))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		if !c.HasDict() {
			t.Error("HasDict() = false, want true")
		}

		if c.DictSize() != len(dict) {
			t.Errorf("DictSize() = %d, want %d", c.DictSize(), len(dict))
		}
	})
}

func TestCompressor_StreamingRoundTrip(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	testData := bytes.Repeat([]byte("streaming test data "), 1000)

	// Compress with writer
	var compressed bytes.Buffer
	w, err := c.Writer(&compressed)
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, err := w.Write(testData); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Decompress with reader
	r, err := c.Reader(&compressed)
	if err != nil {
		t.Fatalf("Reader() error = %v", err)
	}
	defer r.Close()

	var decompressed bytes.Buffer
	if _, err := decompressed.ReadFrom(r); err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

	if !bytes.Equal(decompressed.Bytes(), testData) {
		t.Error("streaming round trip failed")
	}
}

func TestCompressor_CompressTo(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	testData := []byte("test data for CompressTo")
	dst := make([]byte, 0, 100)

	compressed, err := c.CompressTo(dst, testData)
	if err != nil {
		t.Fatalf("CompressTo() error = %v", err)
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress() error = %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Error("CompressTo round trip failed")
	}
}

func generateSampleData(count int) [][]byte {
	samples := make([][]byte, count)
	paths := []string{
		"/usr/local/bin/",
		"/home/user/documents/",
		"/var/log/",
		"/etc/",
		"/opt/app/",
		"/System/Library/Frameworks/",
		"/Applications/",
		"/private/var/folders/",
	}
	files := []string{
		"main.go",
		"config.yaml",
		"README.md",
		"server.log",
		"data.json",
		"index.html",
		"package.json",
		"Makefile",
	}
	exts := []string{".go", ".yaml", ".md", ".log", ".json", ".txt", ".xml"}

	for i := range samples {
		var sb strings.Builder
		// Generate more content per sample
		for j := 0; j < 50; j++ {
			sb.WriteString(paths[(i+j)%len(paths)])
			sb.WriteString(files[(i+j)%len(files)])
			sb.WriteString(exts[(i+j)%len(exts)])
			sb.WriteString(" 4096 ")
			sb.WriteString("drwxr-xr-x")
			sb.WriteString(" 2024-01-15T10:30:00Z")
			sb.WriteString("\n")
		}
		samples[i] = []byte(sb.String())
	}
	return samples
}

func BenchmarkCompressor_Compress(b *testing.B) {
	c, _ := New()
	data := bytes.Repeat([]byte("benchmark data for compression testing "), 100)

	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Compress(data)
	}
}

func BenchmarkCompressor_Decompress(b *testing.B) {
	c, _ := New()
	data := bytes.Repeat([]byte("benchmark data for compression testing "), 100)
	compressed, _ := c.Compress(data)

	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Decompress(compressed)
	}
}
