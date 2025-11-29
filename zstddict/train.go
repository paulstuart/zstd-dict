package zstddict

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/dict"
	"github.com/klauspost/compress/zstd"
)

// TrainDictOptions configures dictionary training.
type TrainDictOptions struct {
	// MaxDictSize is the maximum dictionary size in bytes.
	// If 0, a default size is used.
	MaxDictSize int
	// ID is an optional dictionary ID (default: random).
	ID uint32
	// Level is the encoder level to optimize for (default: best compression).
	Level zstd.EncoderLevel
}

// TrainDict trains a zstd dictionary from the provided samples.
// The samples should be representative of the data that will be compressed.
// For small data (the primary use case for dictionaries), provide many
// small samples rather than a few large ones.
func TrainDict(samples [][]byte, opts *TrainDictOptions) ([]byte, error) {
	if len(samples) == 0 {
		return nil, errors.New("no samples provided for training")
	}

	dictOpts := dict.Options{
		HashBytes:      6,
		ZstdDictCompat: true, // Compatible with standard zstd
	}

	if opts != nil {
		if opts.MaxDictSize > 0 {
			dictOpts.MaxDictSize = opts.MaxDictSize
		}
		dictOpts.ZstdDictID = opts.ID
		dictOpts.ZstdLevel = opts.Level
	}

	if dictOpts.MaxDictSize == 0 {
		dictOpts.MaxDictSize = 32 * 1024 // 32KB default
	}

	return dict.BuildZstdDict(samples, dictOpts)
}

// TrainDictFromFiles trains a dictionary from all files in the given directory.
// It reads each file as a sample for training.
func TrainDictFromFiles(dir string, opts *TrainDictOptions) ([]byte, error) {
	var samples [][]byte

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		samples = append(samples, data)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return TrainDict(samples, opts)
}

// TrainDictFromReader trains a dictionary from samples read from individual byte slices.
// This is useful when samples are generated programmatically.
func TrainDictFromReader(sampleFn func() ([]byte, bool), opts *TrainDictOptions) ([]byte, error) {
	var samples [][]byte

	for {
		sample, ok := sampleFn()
		if !ok {
			break
		}
		samples = append(samples, sample)
	}

	return TrainDict(samples, opts)
}

// SaveDict saves a dictionary to the specified file path.
func SaveDict(dict []byte, path string) error {
	return os.WriteFile(path, dict, 0644)
}

// LoadDict loads a dictionary from the specified file path.
func LoadDict(path string) ([]byte, error) {
	return os.ReadFile(path)
}
