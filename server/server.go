// Package server implements the FileListService gRPC server.
package server

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	pb "github.com/paulstuart/zstd-dict/proto/filelist"
	"google.golang.org/protobuf/proto"
)

// FileListServer implements the FileListService.
type FileListServer struct {
	pb.UnimplementedFileListServiceServer
}

// New creates a new FileListServer.
func New() *FileListServer {
	return &FileListServer{}
}

// ListFiles walks the directory tree and returns file information.
func (s *FileListServer) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	root := req.GetPath()
	if root == "" {
		root = "."
	}

	// Resolve to absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var files []*pb.FileInfo
	maxDepth := int(req.GetMaxDepth())

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip permission errors and other access issues
			if os.IsPermission(err) {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			return err
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate relative path and depth
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		// Skip root itself
		if relPath == "." {
			return nil
		}

		// Check depth limit
		if maxDepth > 0 {
			depth := len(filepath.SplitList(relPath))
			if depth > maxDepth {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			// Skip files we can't stat
			return nil
		}

		files = append(files, &pb.FileInfo{
			Path:    relPath,
			Name:    d.Name(),
			Size:    info.Size(),
			Mode:    uint32(info.Mode()),
			ModTime: info.ModTime().Unix(),
			IsDir:   d.IsDir(),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.ListFilesResponse{
		Root:       absRoot,
		Files:      files,
		TotalCount: int64(len(files)),
	}, nil
}

// GenerateSamples generates sample file listing data for dictionary training.
// It walks the given directories and produces serialized FileInfo messages.
func GenerateSamples(dirs []string, maxSamples int) ([][]byte, error) {
	var samples [][]byte

	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}

		err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			if len(samples) >= maxSamples {
				return fs.SkipAll
			}

			relPath, err := filepath.Rel(absDir, path)
			if err != nil || relPath == "." {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}

			// Create a sample that mimics the wire format
			fi := &pb.FileInfo{
				Path:    relPath,
				Name:    d.Name(),
				Size:    info.Size(),
				Mode:    uint32(info.Mode()),
				ModTime: info.ModTime().Unix(),
				IsDir:   d.IsDir(),
			}

			data, err := marshalFileInfo(fi)
			if err != nil {
				return nil
			}

			samples = append(samples, data)
			return nil
		})
		if err != nil {
			continue
		}
	}

	return samples, nil
}

func marshalFileInfo(fi *pb.FileInfo) ([]byte, error) {
	// Use protobuf marshaling to get realistic wire format samples
	return proto.Marshal(fi)
}

// GenerateResponseSamples generates sample ListFilesResponse data for training.
// This produces larger samples that include multiple files per response.
func GenerateResponseSamples(dirs []string, filesPerSample, maxSamples int) ([][]byte, error) {
	var samples [][]byte

	for _, dir := range dirs {
		if len(samples) >= maxSamples {
			break
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}

		var files []*pb.FileInfo
		err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			relPath, err := filepath.Rel(absDir, path)
			if err != nil || relPath == "." {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}

			files = append(files, &pb.FileInfo{
				Path:    relPath,
				Name:    d.Name(),
				Size:    info.Size(),
				Mode:    uint32(info.Mode()),
				ModTime: info.ModTime().Unix(),
				IsDir:   d.IsDir(),
			})

			// Create a sample when we have enough files
			if len(files) >= filesPerSample {
				resp := &pb.ListFilesResponse{
					Root:       absDir,
					Files:      files,
					TotalCount: int64(len(files)),
				}
				data, err := proto.Marshal(resp)
				if err == nil {
					samples = append(samples, data)
				}
				files = nil // Reset for next sample
			}

			if len(samples) >= maxSamples {
				return fs.SkipAll
			}

			return nil
		})
		if err != nil {
			continue
		}

		// Handle remaining files
		if len(files) > 0 && len(samples) < maxSamples {
			resp := &pb.ListFilesResponse{
				Root:       absDir,
				Files:      files,
				TotalCount: int64(len(files)),
			}
			data, err := proto.Marshal(resp)
			if err == nil {
				samples = append(samples, data)
			}
		}
	}

	return samples, nil
}

// TrainFromDirectory creates training samples by walking a directory tree.
func TrainFromDirectory(root string) ([][]byte, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}

	return GenerateResponseSamples([]string{root}, 50, 200)
}
