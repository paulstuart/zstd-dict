// Package grpccodec provides gRPC compression implementations using zstd.
//
// It supports both plain zstd compression and dictionary-enhanced compression.
// The dictionary-based compressor can significantly improve compression ratios
// for small, repetitive data patterns common in gRPC messages.
//
// Usage with dictionary:
//
//	dict, _ := os.ReadFile("my.dict")
//	compressor := grpccodec.NewZstdDict(dict)
//	grpc.Dial(addr, grpc.WithDefaultCallOptions(grpc.UseCompressor(compressor.Name())))
//
// Alternative: Explicit payload compression (not using gRPC's compressor interface)
// can be implemented by compressing message bytes before sending and decompressing
// after receiving. This gives more control but requires manual integration:
//
//	// Client side - compress before sending
//	compressed, _ := zstddict.Compress(payload)
//	client.Send(&Message{Data: compressed, Compressed: true})
//
//	// Server side - decompress after receiving
//	if msg.Compressed {
//	    payload, _ = zstddict.Decompress(msg.Data)
//	}
package grpccodec

import (
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc/encoding"
)

const (
	// NameZstd is the compressor name for plain zstd compression.
	NameZstd = "zstd"
	// NameZstdDict is the compressor name for dictionary-enhanced zstd compression.
	NameZstdDict = "zstd-dict"
)

func init() {
	// Register plain zstd compressor by default
	encoding.RegisterCompressor(NewZstd())
}

// Zstd implements the grpc/encoding.Compressor interface using zstd.
type Zstd struct {
	name string
	dict []byte

	encoderPool sync.Pool
	decoderPool sync.Pool
}

// NewZstd creates a new zstd compressor without dictionary support.
func NewZstd() *Zstd {
	z := &Zstd{name: NameZstd}
	z.initPools()
	return z
}

// NewZstdDict creates a new zstd compressor with dictionary support.
// The dictionary should be pre-trained on representative data.
func NewZstdDict(dict []byte) *Zstd {
	z := &Zstd{
		name: NameZstdDict,
		dict: dict,
	}
	z.initPools()
	return z
}

func (z *Zstd) initPools() {
	z.encoderPool = sync.Pool{
		New: func() any {
			var enc *zstd.Encoder
			var err error
			if z.dict != nil {
				enc, err = zstd.NewWriter(nil,
					zstd.WithEncoderDict(z.dict),
					zstd.WithEncoderConcurrency(1),
				)
			} else {
				enc, err = zstd.NewWriter(nil,
					zstd.WithEncoderConcurrency(1),
				)
			}
			if err != nil {
				return nil
			}
			return enc
		},
	}

	z.decoderPool = sync.Pool{
		New: func() any {
			var dec *zstd.Decoder
			var err error
			if z.dict != nil {
				dec, err = zstd.NewReader(nil, zstd.WithDecoderDicts(z.dict))
			} else {
				dec, err = zstd.NewReader(nil)
			}
			if err != nil {
				return nil
			}
			return dec
		},
	}
}

// Name returns the name of the compressor.
func (z *Zstd) Name() string {
	return z.name
}

// Compress implements encoding.Compressor.
func (z *Zstd) Compress(w io.Writer) (io.WriteCloser, error) {
	enc := z.encoderPool.Get().(*zstd.Encoder)
	if enc == nil {
		// Fallback: create new encoder
		var err error
		if z.dict != nil {
			enc, err = zstd.NewWriter(w, zstd.WithEncoderDict(z.dict))
		} else {
			enc, err = zstd.NewWriter(w)
		}
		if err != nil {
			return nil, err
		}
		return enc, nil
	}

	enc.Reset(w)
	return &pooledEncoder{enc: enc, pool: &z.encoderPool}, nil
}

// Decompress implements encoding.Compressor.
func (z *Zstd) Decompress(r io.Reader) (io.Reader, error) {
	dec := z.decoderPool.Get().(*zstd.Decoder)
	if dec == nil {
		// Fallback: create new decoder
		if z.dict != nil {
			return zstd.NewReader(r, zstd.WithDecoderDicts(z.dict))
		}
		return zstd.NewReader(r)
	}

	if err := dec.Reset(r); err != nil {
		z.decoderPool.Put(dec)
		return nil, err
	}
	return &pooledDecoder{dec: dec, pool: &z.decoderPool}, nil
}

// pooledEncoder wraps a zstd.Encoder to return it to the pool on Close.
type pooledEncoder struct {
	enc  *zstd.Encoder
	pool *sync.Pool
}

func (p *pooledEncoder) Write(data []byte) (int, error) {
	return p.enc.Write(data)
}

func (p *pooledEncoder) Close() error {
	err := p.enc.Close()
	p.pool.Put(p.enc)
	return err
}

// pooledDecoder wraps a zstd.Decoder to return it to the pool when done.
type pooledDecoder struct {
	dec  *zstd.Decoder
	pool *sync.Pool
}

func (p *pooledDecoder) Read(data []byte) (int, error) {
	n, err := p.dec.Read(data)
	if err == io.EOF {
		p.pool.Put(p.dec)
	}
	return n, err
}

// Register registers both the plain and dictionary-based zstd compressors.
// The dictionary compressor requires the dictionary to be passed.
func Register(dict []byte) {
	encoding.RegisterCompressor(NewZstd())
	if dict != nil {
		encoding.RegisterCompressor(NewZstdDict(dict))
	}
}
