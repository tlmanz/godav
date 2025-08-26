// Package nextcloud provides a high-level client for Nextcloud WebDAV operations,
// including chunked uploads that bypass proxy body-size limits.
package godav

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gowebdav "github.com/studio-b12/gowebdav"
)

// Client wraps a gowebdav.Client with Nextcloud-specific operations.
type Client struct {
	*gowebdav.Client
	username string
	verbose  bool
}

// ProgressInfo contains detailed progress information for uploads.
type ProgressInfo struct {
	Filename    string  // Name of the file being uploaded
	Current     int64   // Bytes uploaded so far
	Total       int64   // Total file size in bytes
	Percentage  float64 // Upload progress as percentage (0.0 to 100.0)
	ChunkIndex  int     // Current chunk number (0-based)
	TotalChunks int     // Total number of chunks
}

// Config holds options for upload operations.
type Config struct {
	ChunkSize    int64                   // Chunk size in bytes (default 10MB)
	SkipExisting bool                    // Skip files that exist with same size
	Verbose      bool                    // Enable verbose logging
	ProgressFunc func(info ProgressInfo) // Progress callback with detailed info
}

// DefaultConfig returns sensible defaults for upload operations.
func DefaultConfig() *Config {
	return &Config{
		ChunkSize:    10 * 1024 * 1024, // 10MB
		SkipExisting: true,
		Verbose:      false,
	}
}

// NewClient creates a new Nextcloud WebDAV client.
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		Client:   gowebdav.NewClient(baseURL, username, password),
		username: username,
		verbose:  false,
	}
}

// SetVerbose enables or disables verbose logging.
func (c *Client) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// UploadFile uploads a single file using Nextcloud's chunked upload protocol.
// The dstPath is relative to the user's files directory.
func (c *Client) UploadFile(localPath, dstPath string, config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	// Convert to Nextcloud files path
	finalPath := c.toFilesPath(dstPath)

	// Skip if exists with same size
	if config.SkipExisting {
		if info, err := c.Stat(finalPath); err == nil && !info.IsDir() {
			if localInfo, lerr := os.Stat(localPath); lerr == nil && info.Size() == localInfo.Size() {
				if config.Verbose || c.verbose {
					log.Printf("Skip unchanged: %s", finalPath)
				}
				return nil
			}
		}
	}

	// Ensure destination directory exists
	if dir := c.dirOf(finalPath); dir != "" {
		if err := c.MkdirAll(dir, 0o755); err != nil && !c.isAlreadyExists(err) {
			if config.Verbose || c.verbose {
				log.Printf("mkdir final dir %s: %v", dir, err)
			}
		}
	}

	return c.uploadChunked(localPath, finalPath, config)
}

// UploadDir uploads a directory recursively using chunked uploads.
func (c *Client) UploadDir(localDir, dstDir string, config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	return filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if localPath == localDir {
			return nil
		}

		// Get relative path
		rel, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return err
		}

		remotePath := c.pathJoin(dstDir, filepath.ToSlash(rel))

		if info.IsDir() {
			// Create directory
			finalPath := c.toFilesPath(remotePath)
			if err := c.MkdirAll(finalPath, 0o755); err != nil && !c.isAlreadyExists(err) {
				if config.Verbose || c.verbose {
					log.Printf("mkdir %s: %v", finalPath, err)
				}
			}
		} else {
			// Upload file
			if err := c.UploadFile(localPath, remotePath, config); err != nil {
				log.Printf("upload %s: %v", remotePath, err)
			}
		}

		return nil
	})
}

// uploadChunked performs the Nextcloud chunked upload protocol:
// 1) MKCOL /uploads/<user>/<upload-id>
// 2) PUT /uploads/<user>/<upload-id>/<offset> for each chunk
// 3) MOVE /uploads/<user>/<upload-id>/.file -> /files/<user>/<dst>
func (c *Client) uploadChunked(localPath, finalPath string, config *Config) error {
	// Prepare upload collection
	uploadID := c.newUploadID()
	uploadBase := c.pathJoinMany("uploads", c.username, uploadID)

	if err := c.MkdirAll(uploadBase, 0o755); err != nil && !c.isAlreadyExists(err) {
		return fmt.Errorf("mkcol %s: %w", uploadBase, err)
	}

	// Open and get file info
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}
	total := fi.Size()

	// Upload chunks
	chunkSize := config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 10 * 1024 * 1024
	}

	buf := make([]byte, int(chunkSize))
	var sent int64
	totalChunks := int((total + chunkSize - 1) / chunkSize) // Ceiling division
	chunkIndex := 0

	for offset := int64(0); offset < total; offset += int64(len(buf)) {
		want := int64(len(buf))
		if remain := total - offset; remain < want {
			want = remain
		}

		n, rerr := io.ReadFull(f, buf[:int(want)])
		if rerr != nil && rerr != io.ErrUnexpectedEOF && rerr != io.EOF {
			return fmt.Errorf("read chunk at %d: %w", offset, rerr)
		}

		chunkPath := c.pathJoin(uploadBase, strconv.FormatInt(offset, 10))
		if err := c.Write(chunkPath, buf[:n], 0o644); err != nil {
			return fmt.Errorf("put chunk %s: %w", chunkPath, err)
		}

		sent += int64(n)
		chunkIndex++

		// Call progress callbacks if provided
		percentage := float64(sent) / float64(total) * 100.0

		if config.ProgressFunc != nil {
			info := ProgressInfo{
				Filename:    filepath.Base(localPath),
				Current:     sent,
				Total:       total,
				Percentage:  percentage,
				ChunkIndex:  chunkIndex - 1, // 0-based
				TotalChunks: totalChunks,
			}
			config.ProgressFunc(info)
		}

		if config.Verbose || c.verbose {
			log.Printf("chunk %s: +%d bytes (%d/%d, %.1f%%)", chunkPath, n, sent, total, percentage)
		}
	}

	// Finalize: MOVE to final destination
	c.SetHeader("OC-Total-Length", strconv.FormatInt(total, 10))
	defer c.SetHeader("OC-Total-Length", "")

	src := c.pathJoin(uploadBase, ".file")
	if err := c.Rename(src, finalPath, true); err != nil {
		_ = c.RemoveAll(uploadBase)
		return fmt.Errorf("finalize move %s -> %s: %w", src, finalPath, err)
	}

	// Cleanup
	_ = c.RemoveAll(uploadBase)

	if config.Verbose || c.verbose {
		log.Printf("Uploaded (chunked): %s", finalPath)
	}

	return nil
}

// Helper methods

func (c *Client) toFilesPath(dstPath string) string {
	dstPath = strings.TrimPrefix(dstPath, "/")
	if strings.HasPrefix(dstPath, "files/") {
		return strings.TrimPrefix(dstPath, "/")
	}
	return c.pathJoinMany("files", c.username, dstPath)
}

func (c *Client) pathJoin(a, b string) string {
	a = strings.TrimSuffix(a, "/")
	b = strings.TrimPrefix(b, "/")
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "/" + b
}

func (c *Client) pathJoinMany(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" {
			out = strings.Trim(p, "/")
			continue
		}
		out = out + "/" + strings.Trim(p, "/")
	}
	return out
}

func (c *Client) dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

func (c *Client) isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "exists") || strings.Contains(msg, "405") || strings.Contains(msg, "409")
}

func (c *Client) newUploadID() string {
	return fmt.Sprintf("web-file-upload-%d", time.Now().UnixNano())
}
