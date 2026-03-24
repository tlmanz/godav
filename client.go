// Package godav provides a high-level client for Nextcloud WebDAV operations,
// including chunked uploads that bypass proxy body-size limits.
//
// The library is organized into several modules for better maintainability:
//   - client.go: Core client functionality and upload methods
//   - types.go: Type definitions, constants, and configuration structures
//   - chunked_upload.go: Chunked upload implementation with retry logic
//   - upload_controller.go: Pause/resume/cancel functionality for uploads
//   - upload_manager.go: Multi-session upload coordination and management
//   - checkpoint.go: Upload resumption and checkpoint persistence
//   - buffer_pool.go: Memory-efficient buffer management
//   - utils.go: Helper functions and utilities
//
// Basic usage:
//
//	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
//	config := godav.DefaultConfig()
//	config.Verbose = true
//
//	// Upload a single file
//	err := client.UploadFile("/path/to/local/file.txt", "remote/path/file.txt", config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Advanced features include pause/resume support, progress tracking, event handling,
// and automatic checkpoint saving for large file uploads.
package godav

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	gowebdav "github.com/studio-b12/gowebdav"
)

// Client wraps a gowebdav.Client with Nextcloud-specific operations.
// It provides high-level methods for uploading files and directories
// with support for chunked uploads, progress tracking, and pause/resume functionality.
type Client struct {
	*gowebdav.Client
	username string
	config   *Config
	hdrMu    sync.Mutex
}

// NewClient creates a new Nextcloud WebDAV client.
//
// Parameters:
//   - baseURL: The base URL of the Nextcloud WebDAV endpoint (e.g., "https://nextcloud.example.com/remote.php/dav/")
//   - username: The username for authentication
//   - password: The password or app password for authentication
//
// Returns a configured Client ready for upload operations.
//
// Example:
//
//	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		Client:   gowebdav.NewClient(baseURL, username, password),
		username: username,
		config:   DefaultConfig(),
	}
}

// SetVerbose enables or disables verbose logging for upload operations.
// When enabled, the client will log detailed information about upload progress,
// chunk operations, and directory creation.
func (c *Client) SetVerbose(verbose bool) {
	c.config.Verbose = verbose
}

// SetConfig replaces the client's default configuration used by methods that
// do not take an explicit config parameter (e.g., UploadFile, UploadDir, UploadManager flows).
// The provided config is validated and sanitized.
// Not safe to change concurrently with ongoing uploads on the same client.
func (c *Client) SetConfig(cfg *Config) {
	c.config = validateConfigWith(cfg)
}

// UploadFile uploads a single file using Nextcloud's chunked upload protocol.
// The dstPath is relative to the user's files directory.
//
// This method uses chunked uploads to bypass proxy body-size limits and provides
// better reliability for large files. It supports pause/resume functionality,
// progress tracking, and automatic retry logic for failed chunks.
//
// Parameters:
//   - localPath: Local file path to upload
//   - dstPath: Remote destination path (relative to user's files directory)
//   - config: Upload configuration (use DefaultConfig() for sensible defaults)
//
// Returns an error if the upload fails. Use UploadError type assertion for detailed error information.
//
// Example:
//
//	config := godav.DefaultConfig()
//	config.Verbose = true
//	err := client.UploadFile("/path/to/file.txt", "remote/file.txt", config)
//	if err != nil {
//		var uploadErr *godav.UploadError
//		if errors.As(err, &uploadErr) {
//			fmt.Printf("Upload failed: %s (retries: %d)\n", uploadErr.Op, uploadErr.Retries)
//		}
//	}
func (c *Client) UploadFile(localPath, dstPath string) error {
	return c.uploadFileCore(context.Background(), localPath, dstPath, c.config)
}

// UploadFileWithConfig uploads a single file using the provided config (does not mutate the client's default config).
// Safe to call concurrently with other uploads since c.config is not touched.
func (c *Client) UploadFileWithConfig(localPath, dstPath string, cfg *Config) error {
	return c.uploadFileCore(context.Background(), localPath, dstPath, validateConfigWith(cfg))
}

// UploadFileWithContext uploads a single file with context support for cancellation and timeouts.
// This method provides the same functionality as UploadFile but allows for cancellation
// and timeout control through the provided context.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - localPath: Local file path to upload
//   - dstPath: Remote destination path (relative to user's files directory)
//   - config: Upload configuration
//
// The context is checked at the beginning of the upload operation. For more granular
// cancellation control during upload, use the pause/resume functionality via UploadController.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
//	defer cancel()
//	err := client.UploadFileWithContext(ctx, localPath, remotePath, config)
//	if err == context.DeadlineExceeded {
//		fmt.Println("Upload timed out")
//	}
func (c *Client) UploadFileWithContext(ctx context.Context, localPath, dstPath string) error {
	return c.uploadFileCore(ctx, localPath, dstPath, c.config)
}

// UploadFileWithContextWithConfig uploads with context and the provided config (does not mutate the client's default config).
// Safe to call concurrently with other uploads since c.config is not touched.
func (c *Client) UploadFileWithContextWithConfig(ctx context.Context, localPath, dstPath string, cfg *Config) error {
	return c.uploadFileCore(ctx, localPath, dstPath, validateConfigWith(cfg))
}

// uploadFileCore contains the core logic for file upload using the provided cfg.
func (c *Client) uploadFileCore(ctx context.Context, localPath, dstPath string, cfg *Config) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Emit upload started event
	filename := filepath.Base(localPath)
	c.emitEvent(cfg, EventUploadStarted, filename, dstPath, "Upload started", nil)

	// Convert to Nextcloud files path and validate
	cleaned := c.sanitizeRemotePath(dstPath)
	if cleaned == "" {
		return fmt.Errorf("invalid remote path")
	}
	finalPath := c.pathJoinMany("files", c.username, cleaned)

	// Skip if exists with same size
	if cfg.SkipExisting {
		if info, err := c.Stat(finalPath); err == nil && !info.IsDir() {
			if localInfo, lerr := os.Stat(localPath); lerr == nil && info.Size() == localInfo.Size() {
				if cfg.Verbose {
					log.Printf("Skip unchanged: %s", finalPath)
				}
				c.emitEvent(cfg, EventUploadSkipped, filename, dstPath, "File already exists with same size", nil)
				return nil
			}
		}
	}

	// Ensure destination directory exists
	if dir := c.dirOf(finalPath); dir != "" {
		if err := c.MkdirAll(dir, 0o755); err != nil && !c.isAlreadyExists(err) {
			if cfg.Verbose {
				log.Printf("mkdir final dir %s: %v", dir, err)
			}
		}
	}

	err := c.uploadChunked(ctx, localPath, finalPath, cfg)
	if err != nil {
		c.emitEvent(cfg, EventUploadFailed, filename, dstPath, "Upload failed", err)
		return err
	}

	c.emitEvent(cfg, EventUploadComplete, filename, dstPath, "Upload completed successfully", nil)
	return nil
}

// UploadDir uploads a directory recursively using chunked uploads.
// Errors from individual file uploads are collected and returned as a combined error;
// the walk continues even when a file fails.
func (c *Client) UploadDir(localDir, dstDir string) error {
	c.config = c.validateConfig()

	var uploadErrs []error
	walkErr := filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
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
				if c.config.Verbose {
					log.Printf("mkdir %s: %v", finalPath, err)
				}
			}
		} else {
			// Upload file; collect error but continue the walk
			if err := c.UploadFile(localPath, remotePath); err != nil {
				log.Printf("upload %s: %v", remotePath, err)
				uploadErrs = append(uploadErrs, fmt.Errorf("%s: %w", remotePath, err))
			}
		}

		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return errors.Join(uploadErrs...)
}
