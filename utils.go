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
	// path.Clean on an absolute path prevents traversal beyond root;
	// stripping the leading slash gives a safe relative path.
	cleaned := path.Clean("/" + p)
	return strings.TrimPrefix(cleaned, "/")
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

// emitEvent calls the event callback if configured.
// cfg is passed explicitly so callers can use a local copy and avoid races on c.config.
func (c *Client) emitEvent(cfg *Config, event UploadEvent, filename, path, message string, err error) {
	if cfg == nil || cfg.EventFunc == nil {
		return
	}
	sessionID := ""
	if cfg.Controller != nil {
		sessionID = cfg.Controller.sessionID
	}
	info := EventInfo{
		Event:     event,
		Filename:  filename,
		Path:      path,
		Message:   message,
		Error:     err,
		SessionID: sessionID,
	}
	cfg.EventFunc(info)
}

// validateConfigWith returns a validated copy of cfg without mutating the original.
func validateConfigWith(cfg *Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}
	out := *cfg // shallow copy; function pointers are shared intentionally
	if out.ChunkSize <= 0 {
		out.ChunkSize = 10 * 1024 * 1024
	}
	if out.ChunkSize < 1024 {
		out.ChunkSize = 1024
	}
	if out.ChunkSize > 1024*1024*1024 {
		out.ChunkSize = 1024 * 1024 * 1024
	}
	if out.MaxRetries < 0 {
		out.MaxRetries = 0
	}
	if out.MaxRetries > 10 {
		out.MaxRetries = 10
	}
	if out.BufferPool != nil && out.BufferPool.size != out.ChunkSize {
		out.BufferPool = NewBufferPool(out.ChunkSize, 4)
	}
	return &out
}

// validateConfig validates and sanitizes c.config in-place (used by SetConfig and UploadDir).
func (c *Client) validateConfig() *Config {
	c.config = validateConfigWith(c.config)
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
