// Package zstddict provides zstd compression with optional dictionary support.
// It wraps github.com/klauspost/compress/zstd to provide a simple API for
// compressing and decompressing data with pre-trained dictionaries.
package zstddict

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// Compressor provides zstd compression with optional dictionary support.
// It maintains encoder and decoder pools for efficient reuse.
type Compressor struct {
	dict []byte

	encoderPool sync.Pool
	decoderPool sync.Pool
}

// Option configures a Compressor.
type Option func(*Compressor) error

// WithDictBytes loads a dictionary from the provided bytes.
func WithDictBytes(dict []byte) Option {
	return func(c *Compressor) error {
		c.dict = dict
		return nil
	}
}

// WithDictFile loads a dictionary from the specified file path.
func WithDictFile(path string) Option {
	return func(c *Compressor) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		c.dict = data
		return nil
	}
}

// New creates a new Compressor with the given options.
func New(opts ...Option) (*Compressor, error) {
	c := &Compressor{}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	c.encoderPool = sync.Pool{
		New: func() any {
			var enc *zstd.Encoder
			var err error
			if c.dict != nil {
				enc, err = zstd.NewWriter(nil, zstd.WithEncoderDict(c.dict))
			} else {
				enc, err = zstd.NewWriter(nil)
			}
			if err != nil {
				return nil
			}
			return enc
		},
	}

	c.decoderPool = sync.Pool{
		New: func() any {
			var dec *zstd.Decoder
			var err error
			if c.dict != nil {
				dec, err = zstd.NewReader(nil, zstd.WithDecoderDicts(c.dict))
			} else {
				dec, err = zstd.NewReader(nil)
			}
			if err != nil {
				return nil
			}
			return dec
		},
	}

	return c, nil
}

// Compress compresses the input data using zstd with the configured dictionary.
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	enc := c.encoderPool.Get().(*zstd.Encoder)
	if enc == nil {
		return nil, errors.New("failed to get encoder from pool")
	}
	defer c.encoderPool.Put(enc)

	return enc.EncodeAll(data, nil), nil
}

// CompressTo compresses the input data and appends to dst.
func (c *Compressor) CompressTo(dst, data []byte) ([]byte, error) {
	enc := c.encoderPool.Get().(*zstd.Encoder)
	if enc == nil {
		return nil, errors.New("failed to get encoder from pool")
	}
	defer c.encoderPool.Put(enc)

	return enc.EncodeAll(data, dst), nil
}

// Decompress decompresses the input data using zstd with the configured dictionary.
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	dec := c.decoderPool.Get().(*zstd.Decoder)
	if dec == nil {
		return nil, errors.New("failed to get decoder from pool")
	}
	defer c.decoderPool.Put(dec)

	return dec.DecodeAll(data, nil)
}

// DecompressTo decompresses the input data and appends to dst.
func (c *Compressor) DecompressTo(dst, data []byte) ([]byte, error) {
	dec := c.decoderPool.Get().(*zstd.Decoder)
	if dec == nil {
		return nil, errors.New("failed to get decoder from pool")
	}
	defer c.decoderPool.Put(dec)

	return dec.DecodeAll(data, dst)
}

// Writer returns a streaming zstd writer that writes compressed data to w.
func (c *Compressor) Writer(w io.Writer) (*zstd.Encoder, error) {
	if c.dict != nil {
		return zstd.NewWriter(w, zstd.WithEncoderDict(c.dict))
	}
	return zstd.NewWriter(w)
}

// Reader returns a streaming zstd reader that decompresses data from r.
func (c *Compressor) Reader(r io.Reader) (*zstd.Decoder, error) {
	if c.dict != nil {
		return zstd.NewReader(r, zstd.WithDecoderDicts(c.dict))
	}
	return zstd.NewReader(r)
}

// HasDict returns true if the compressor has a dictionary loaded.
func (c *Compressor) HasDict() bool {
	return c.dict != nil
}

// DictSize returns the size of the loaded dictionary in bytes.
func (c *Compressor) DictSize() int {
	return len(c.dict)
}
