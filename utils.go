// Package godav - Utility functions and helpers
//
// This file contains helper functions used throughout the godav library,
// including path manipulation, configuration validation, event emission,
// and other utility functions that support the core upload functionality.
//
// Functions include:
//   - Path manipulation and joining
//   - Configuration validation and sanitization
//   - Event emission for upload lifecycle
//   - Error checking utilities
//   - Upload ID generation
package godav

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// Helper methods

// sanitizeRemotePath cleans a user-provided remote path and prevents traversal.
// It rejects any path containing ".." segments after cleaning.
func (c *Client) sanitizeRemotePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	// If user passed a full files/.. path, strip the leading files/<anything>/ prefix.
	if strings.HasPrefix(p, "files/") {
		parts := strings.Split(p, "/")
		// drop first two segments if available (files/<user>), keep rest
		if len(parts) >= 3 {
			p = strings.Join(parts[2:], "/")
		} else {
			p = ""
		}
	}
	// Clean to collapse any ./ or ../ segments
	cleaned := path.Clean("/" + p) // ensure absolute for Clean semantics
	cleaned = strings.TrimPrefix(cleaned, "/")
	// Reject traversal
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		// Drop to empty to avoid writing outside user's files root
		cleaned = ""
	}
	return cleaned
}

func (c *Client) toFilesPath(dstPath string) string {
	dst := c.sanitizeRemotePath(dstPath)
	return c.pathJoinMany("files", c.username, dst)
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

// emitEvent calls the event callback if configured
func (c *Client) emitEvent(event UploadEvent, filename, path, message string, err error) {
	if c.config.EventFunc != nil {
		sessionID := ""
		if c.config.Controller != nil {
			sessionID = c.config.Controller.sessionID
		}

		info := EventInfo{
			Event:     event,
			Filename:  filename,
			Path:      path,
			Message:   message,
			Error:     err,
			SessionID: sessionID,
		}
		c.config.EventFunc(info)
	}
}

// validateConfig validates and sanitizes the configuration
func (c *Client) validateConfig() *Config {
	if c.config == nil {
		return DefaultConfig()
	}

	// Ensure chunk size is reasonable
	if c.config.ChunkSize <= 0 {
		c.config.ChunkSize = 10 * 1024 * 1024 // 10MB default
	}

	// Minimum chunk size of 1KB to avoid excessive requests
	if c.config.ChunkSize < 1024 {
		c.config.ChunkSize = 1024
	}

	// Maximum chunk size of 1GB to avoid memory issues
	if c.config.ChunkSize > 1024*1024*1024 {
		c.config.ChunkSize = 1024 * 1024 * 1024
	}

	// Ensure max retries is reasonable
	if c.config.MaxRetries < 0 {
		c.config.MaxRetries = 0
	}
	if c.config.MaxRetries > 10 {
		c.config.MaxRetries = 10 // Cap at 10 retries
	}

	// Ensure buffer pool (if provided) matches chunk size to avoid reallocation churn
	if c.config.BufferPool != nil {
		if c.config.BufferPool.size != c.config.ChunkSize {
			// Recreate a matching pool with a small default size
			c.config.BufferPool = NewBufferPool(c.config.ChunkSize, 4)
		}
	}

	return c.config
}

// calculateChunks returns the number of chunks needed for a file
func calculateChunks(fileSize, chunkSize int64) int {
	return int((fileSize + chunkSize - 1) / chunkSize)
}

// DefaultConfig returns sensible defaults for upload operations.
func DefaultConfig() *Config {
	return &Config{
		ChunkSize:    10 * 1024 * 1024, // 10MB
		SkipExisting: true,
		Verbose:      false,
		MaxRetries:   3,
		BufferPool:   NewBufferPool(10*1024*1024, 4), // Pool of 4 buffers
	}
}
