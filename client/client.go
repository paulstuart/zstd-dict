// Package client provides a gRPC client for the FileListService.
package client

import (
	"context"
	"time"

	pb "github.com/paulstuart/zstd-dict/proto/filelist"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the FileListService gRPC client.
type Client struct {
	conn   *grpc.ClientConn
	client pb.FileListServiceClient
}

// Options configures the client connection.
type Options struct {
	// Address is the server address (host:port).
	Address string
	// Compressor is the name of the compressor to use (e.g., "zstd", "zstd-dict", "gzip").
	Compressor string
	// Timeout is the connection timeout.
	Timeout time.Duration
}

// New creates a new client connection to the FileListService.
func New(opts Options) (*Client, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if opts.Compressor != "" {
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(
			grpc.UseCompressor(opts.Compressor),
		))
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, opts.Address, dialOpts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		client: pb.NewFileListServiceClient(conn),
	}, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// ListFiles requests a directory listing from the server.
func (c *Client) ListFiles(ctx context.Context, path string, maxDepth int32) (*pb.ListFilesResponse, error) {
	return c.client.ListFiles(ctx, &pb.ListFilesRequest{
		Path:     path,
		MaxDepth: maxDepth,
	})
}

// ListFilesWithStats requests a directory listing and returns timing/size statistics.
func (c *Client) ListFilesWithStats(ctx context.Context, path string, maxDepth int32) (*pb.ListFilesResponse, Stats, error) {
	start := time.Now()

	resp, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{
		Path:     path,
		MaxDepth: maxDepth,
	})

	stats := Stats{
		Duration: time.Since(start),
	}

	if err != nil {
		return nil, stats, err
	}

	stats.FileCount = resp.TotalCount

	return resp, stats, nil
}

// Stats contains request statistics.
type Stats struct {
	Duration  time.Duration
	FileCount int64
}
