// Package nextcloud provides a high-level client for Nextcloud WebDAV operations,
// including chunked uploads that bypass proxy body-size limits.
package godav

import (
	"context"
	"encoding/json"
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

// UploadEvent represents different stages of the upload process
type UploadEvent string

const (
	EventUploadStarted  UploadEvent = "upload_started"  // Upload process initiated
	EventChunkUploaded  UploadEvent = "chunk_uploaded"  // Individual chunk uploaded
	EventChunksComplete UploadEvent = "chunks_complete" // All chunks uploaded, before move
	EventMoveStarted    UploadEvent = "move_started"    // Starting final move operation
	EventMoveComplete   UploadEvent = "move_complete"   // Move operation completed
	EventUploadComplete UploadEvent = "upload_complete" // Entire upload process finished
	EventUploadFailed   UploadEvent = "upload_failed"   // Upload failed
	EventUploadSkipped  UploadEvent = "upload_skipped"  // File skipped (already exists)
	EventUploadPaused   UploadEvent = "upload_paused"   // Upload paused
	EventUploadResumed  UploadEvent = "upload_resumed"  // Upload resumed from checkpoint
)

// UploadError represents errors that occur during upload
type UploadError struct {
	Op      string // Operation that failed
	Path    string // File path involved
	Err     error  // Underlying error
	Retries int    // Number of retries attempted
}

func (e *UploadError) Error() string {
	if e.Retries > 0 {
		return fmt.Sprintf("%s %s: %v (after %d retries)", e.Op, e.Path, e.Err, e.Retries)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

func (e *UploadError) Unwrap() error {
	return e.Err
}

// UploadState represents the current state of an upload
type UploadState int

const (
	StateRunning UploadState = iota
	StatePaused
	StateCancelled
)

// UploadController provides pause/resume functionality for uploads
type UploadController struct {
	state    UploadState
	stateCh  chan UploadState
	pauseCh  chan struct{}
	resumeCh chan struct{}
}

// NewUploadController creates a new upload controller
func NewUploadController() *UploadController {
	return &UploadController{
		state:    StateRunning,
		stateCh:  make(chan UploadState, 1),
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}
}

// Pause pauses the upload
func (uc *UploadController) Pause() {
	if uc.state == StateRunning {
		uc.state = StatePaused
		select {
		case uc.pauseCh <- struct{}{}:
		default:
		}
	}
}

// Resume resumes the upload
func (uc *UploadController) Resume() {
	if uc.state == StatePaused {
		uc.state = StateRunning
		select {
		case uc.resumeCh <- struct{}{}:
		default:
		}
	}
}

// Cancel cancels the upload
func (uc *UploadController) Cancel() {
	uc.state = StateCancelled
}

// State returns the current upload state
func (uc *UploadController) State() UploadState {
	return uc.state
}

// Checkpoint represents a resumable upload checkpoint
type Checkpoint struct {
	LocalPath    string    `json:"local_path"`
	RemotePath   string    `json:"remote_path"`
	UploadID     string    `json:"upload_id"`
	FileSize     int64     `json:"file_size"`
	ChunkSize    int64     `json:"chunk_size"`
	BytesUploaded int64    `json:"bytes_uploaded"`
	ChunksUploaded int     `json:"chunks_uploaded"`
	TotalChunks  int       `json:"total_chunks"`
	Timestamp    time.Time `json:"timestamp"`
	Config       *Config   `json:"config,omitempty"`
}

// EventInfo contains information about upload events
type EventInfo struct {
	Event    UploadEvent // Type of event
	Filename string      // Name of the file
	Path     string      // Remote path
	Message  string      // Optional message
	Error    error       // Error if applicable
}

// Config holds options for upload operations.
type Config struct {
	ChunkSize       int64                   // Chunk size in bytes (default 10MB)
	SkipExisting    bool                    // Skip files that exist with same size
	Verbose         bool                    // Enable verbose logging
	ProgressFunc    func(info ProgressInfo) // Progress callback with detailed info
	EventFunc       func(info EventInfo)    // Event callback for upload lifecycle
	MaxRetries      int                     // Maximum retry attempts for failed chunks (default 3)
	BufferPool      *BufferPool             // Optional buffer pool for memory reuse
	Controller      *UploadController       // Upload controller for pause/resume (optional)
	CheckpointFunc  func(cp Checkpoint)     // Checkpoint callback for resume functionality
	ResumeFromCheckpoint *Checkpoint        // Resume from this checkpoint (optional)
}

// BufferPool manages reusable byte buffers to reduce allocations
type BufferPool struct {
	pool chan []byte
	size int64
}

// NewBufferPool creates a new buffer pool with the specified chunk size and pool size
func NewBufferPool(chunkSize int64, poolSize int) *BufferPool {
	return &BufferPool{
		pool: make(chan []byte, poolSize),
		size: chunkSize,
	}
}

// Get retrieves a buffer from the pool or creates a new one
func (bp *BufferPool) Get() []byte {
	select {
	case buf := <-bp.pool:
		return buf
	default:
		return make([]byte, bp.size)
	}
}

// Put returns a buffer to the pool for reuse
func (bp *BufferPool) Put(buf []byte) {
	if int64(len(buf)) != bp.size {
		return // Don't pool buffers of wrong size
	}

	select {
	case bp.pool <- buf:
	default:
		// Pool is full, let GC handle it
	}
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
	return c.UploadFileWithContext(context.Background(), localPath, dstPath, config)
}

// UploadFileWithContext uploads a single file with context support for cancellation.
func (c *Client) UploadFileWithContext(ctx context.Context, localPath, dstPath string, config *Config) error {
	config = c.validateConfig(config)

	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Emit upload started event
	filename := filepath.Base(localPath)
	c.emitEvent(config, EventUploadStarted, filename, dstPath, "Upload started", nil)

	// Convert to Nextcloud files path
	finalPath := c.toFilesPath(dstPath)

	// Skip if exists with same size
	if config.SkipExisting {
		if info, err := c.Stat(finalPath); err == nil && !info.IsDir() {
			if localInfo, lerr := os.Stat(localPath); lerr == nil && info.Size() == localInfo.Size() {
				if config.Verbose || c.verbose {
					log.Printf("Skip unchanged: %s", finalPath)
				}
				c.emitEvent(config, EventUploadSkipped, filename, dstPath, "File already exists with same size", nil)
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

	err := c.uploadChunked(localPath, finalPath, config)
	if err != nil {
		c.emitEvent(config, EventUploadFailed, filename, dstPath, "Upload failed", err)
		return err
	}

	c.emitEvent(config, EventUploadComplete, filename, dstPath, "Upload completed successfully", nil)
	return nil
}

// UploadDir uploads a directory recursively using chunked uploads.
func (c *Client) UploadDir(localDir, dstDir string, config *Config) error {
	config = c.validateConfig(config)

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
	// Cache filename to avoid repeated path.Base calls
	filename := filepath.Base(localPath)

	// Check if resuming from checkpoint
	var uploadID string
	var uploadBase string
	var startOffset int64
	var sent int64
	var chunkIndex int
	
	if config.ResumeFromCheckpoint != nil {
		// Resume from checkpoint
		uploadID = config.ResumeFromCheckpoint.UploadID
		uploadBase = c.pathJoinMany("uploads", c.username, uploadID)
		sent = config.ResumeFromCheckpoint.BytesUploaded
		chunkIndex = config.ResumeFromCheckpoint.ChunksUploaded
		startOffset = int64(chunkIndex) * config.ChunkSize
		
		c.emitEvent(config, EventUploadResumed, filename, finalPath, 
			fmt.Sprintf("Resuming from chunk %d", chunkIndex), nil)
	} else {
		// New upload
		uploadID = c.newUploadID()
		uploadBase = c.pathJoinMany("uploads", c.username, uploadID)
		
		if err := c.MkdirAll(uploadBase, 0o755); err != nil && !c.isAlreadyExists(err) {
			return fmt.Errorf("mkcol %s: %w", uploadBase, err)
		}
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

	// Seek to resume position if needed
	if startOffset > 0 {
		_, err = f.Seek(startOffset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seek to offset %d: %w", startOffset, err)
		}
	}

	// Validate chunk size
	chunkSize := config.ChunkSize

	// Use buffer pool if available, otherwise allocate
	var buf []byte
	if config.BufferPool != nil {
		buf = config.BufferPool.Get()
		defer config.BufferPool.Put(buf)
		// Resize buffer if needed
		if int64(len(buf)) != chunkSize {
			buf = make([]byte, chunkSize)
		}
	} else {
		buf = make([]byte, chunkSize)
	}

	totalChunks := calculateChunks(total, chunkSize)

	for offset := startOffset; offset < total; offset += chunkSize {
		// Check for pause/resume/cancel
		if config.Controller != nil {
			switch config.Controller.State() {
			case StatePaused:
				c.emitEvent(config, EventUploadPaused, filename, finalPath, "Upload paused", nil)
				
				// Save checkpoint
				if config.CheckpointFunc != nil {
					checkpoint := Checkpoint{
						LocalPath:      localPath,
						RemotePath:     finalPath,
						UploadID:       uploadID,
						FileSize:       total,
						ChunkSize:      chunkSize,
						BytesUploaded:  sent,
						ChunksUploaded: chunkIndex,
						TotalChunks:    totalChunks,
						Timestamp:      time.Now(),
						Config:         config,
					}
					config.CheckpointFunc(checkpoint)
				}
				
				// Wait for resume or cancel
				select {
				case <-config.Controller.resumeCh:
					c.emitEvent(config, EventUploadResumed, filename, finalPath, "Upload resumed", nil)
				case <-time.After(time.Hour): // Timeout after 1 hour
					return fmt.Errorf("upload paused timeout")
				}
				
			case StateCancelled:
				// Cleanup and return
				_ = c.RemoveAll(uploadBase)
				return fmt.Errorf("upload cancelled")
			}
		}

		want := chunkSize
		if remain := total - offset; remain < want {
			want = remain
		}

		n, rerr := io.ReadFull(f, buf[:want])
		if rerr != nil && rerr != io.ErrUnexpectedEOF && rerr != io.EOF {
			return fmt.Errorf("read chunk at %d: %w", offset, rerr)
		}

		// Retry logic for chunk upload
		var uploadErr error
		for retry := 0; retry <= config.MaxRetries; retry++ {
			chunkPath := c.pathJoin(uploadBase, strconv.FormatInt(offset, 10))
			uploadErr = c.Write(chunkPath, buf[:n], 0o644)
			if uploadErr == nil {
				break // Success
			}

			if retry < config.MaxRetries {
				if config.Verbose || c.verbose {
					log.Printf("chunk upload retry %d/%d for %s: %v", retry+1, config.MaxRetries, chunkPath, uploadErr)
				}
			}
		}

		if uploadErr != nil {
			return &UploadError{
				Op:      "chunk upload",
				Path:    c.pathJoin(uploadBase, strconv.FormatInt(offset, 10)),
				Err:     uploadErr,
				Retries: config.MaxRetries,
			}
		}

		sent += int64(n)
		chunkIndex++

		// Emit chunk uploaded event
		c.emitEvent(config, EventChunkUploaded, filename, finalPath,
			fmt.Sprintf("Chunk %d/%d uploaded", chunkIndex, totalChunks), nil)

		// Call progress callbacks if provided
		if config.ProgressFunc != nil {
			percentage := float64(sent) / float64(total) * 100.0
			info := ProgressInfo{
				Filename:    filename,
				Current:     sent,
				Total:       total,
				Percentage:  percentage,
				ChunkIndex:  chunkIndex - 1, // 0-based
				TotalChunks: totalChunks,
			}
			config.ProgressFunc(info)
		}

		if config.Verbose || c.verbose {
			percentage := float64(sent) / float64(total) * 100.0
			chunkPath := c.pathJoin(uploadBase, strconv.FormatInt(offset, 10))
			log.Printf("chunk %s: +%d bytes (%d/%d, %.1f%%)", chunkPath, n, sent, total, percentage)
		}

		// Save checkpoint periodically (every 10 chunks)
		if config.CheckpointFunc != nil && chunkIndex%10 == 0 {
			checkpoint := Checkpoint{
				LocalPath:      localPath,
				RemotePath:     finalPath,
				UploadID:       uploadID,
				FileSize:       total,
				ChunkSize:      chunkSize,
				BytesUploaded:  sent,
				ChunksUploaded: chunkIndex,
				TotalChunks:    totalChunks,
				Timestamp:      time.Now(),
				Config:         config,
			}
			config.CheckpointFunc(checkpoint)
		}
	}

	// All chunks uploaded
	c.emitEvent(config, EventChunksComplete, filename, finalPath, "All chunks uploaded", nil)

	// Finalize: MOVE to final destination
	c.emitEvent(config, EventMoveStarted, filename, finalPath, "Starting final move operation", nil)
	c.SetHeader("OC-Total-Length", strconv.FormatInt(total, 10))
	defer c.SetHeader("OC-Total-Length", "")

	src := c.pathJoin(uploadBase, ".file")
	if err := c.Rename(src, finalPath, true); err != nil {
		_ = c.RemoveAll(uploadBase)
		return fmt.Errorf("finalize move %s -> %s: %w", src, finalPath, err)
	}

	c.emitEvent(config, EventMoveComplete, filename, finalPath, "Move operation completed", nil)

	// Cleanup
	_ = c.RemoveAll(uploadBase)

	if config.Verbose || c.verbose {
		log.Printf("Uploaded (chunked): %s", finalPath)
	}

	return nil
}

// validateConfig validates and sanitizes the configuration
func (c *Client) validateConfig(config *Config) *Config {
	if config == nil {
		return DefaultConfig()
	}

	// Ensure chunk size is reasonable
	if config.ChunkSize <= 0 {
		config.ChunkSize = 10 * 1024 * 1024 // 10MB default
	}

	// Minimum chunk size of 1KB to avoid excessive requests
	if config.ChunkSize < 1024 {
		config.ChunkSize = 1024
	}

	// Maximum chunk size of 1GB to avoid memory issues
	if config.ChunkSize > 1024*1024*1024 {
		config.ChunkSize = 1024 * 1024 * 1024
	}

	// Ensure max retries is reasonable
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.MaxRetries > 10 {
		config.MaxRetries = 10 // Cap at 10 retries
	}

	return config
}

// calculateChunks returns the number of chunks needed for a file
func calculateChunks(fileSize, chunkSize int64) int {
	return int((fileSize + chunkSize - 1) / chunkSize)
}

// UploadFileResumable uploads a file with built-in pause/resume support
func (c *Client) UploadFileResumable(localPath, dstPath string, config *Config) (*UploadController, error) {
	if config == nil {
		config = DefaultConfig()
	}
	
	// Create upload controller if not provided
	if config.Controller == nil {
		config.Controller = NewUploadController()
	}
	
	// Upload in a goroutine to enable pause/resume
	go func() {
		err := c.UploadFile(localPath, dstPath, config)
		if err != nil && config.EventFunc != nil {
			filename := filepath.Base(localPath)
			c.emitEvent(config, EventUploadFailed, filename, dstPath, "Upload failed", err)
		}
	}()
	
	return config.Controller, nil
}

// ResumeUpload resumes an upload from a checkpoint
func (c *Client) ResumeUpload(checkpoint Checkpoint, config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}
	
	// Set the checkpoint in config
	config.ResumeFromCheckpoint = &checkpoint
	
	return c.UploadFile(checkpoint.LocalPath, checkpoint.RemotePath, config)
}

// SaveCheckpoint saves a checkpoint to a file (JSON format)
func SaveCheckpoint(checkpoint Checkpoint, filePath string) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	
	return os.WriteFile(filePath, data, 0644)
}

// LoadCheckpoint loads a checkpoint from a file
func LoadCheckpoint(filePath string) (*Checkpoint, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint file: %w", err)
	}
	
	var checkpoint Checkpoint
	err = json.Unmarshal(data, &checkpoint)
	if err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	
	return &checkpoint, nil
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

// emitEvent calls the event callback if configured
func (c *Client) emitEvent(config *Config, event UploadEvent, filename, path, message string, err error) {
	if config.EventFunc != nil {
		info := EventInfo{
			Event:    event,
			Filename: filename,
			Path:     path,
			Message:  message,
			Error:    err,
		}
		config.EventFunc(info)
	}
}
