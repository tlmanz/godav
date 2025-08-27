// Package godav - Type definitions and configuration structures
//
// This file contains all the core type definitions used throughout the godav library,
// including progress tracking, event handling, error types, and configuration options.
package godav

// ProgressInfo contains detailed progress information for uploads.
type ProgressInfo struct {
	Filename    string  // Name of the file being uploaded
	Current     int64   // Bytes uploaded so far
	Total       int64   // Total file size in bytes
	Percentage  float64 // Upload progress as percentage (0.0 to 100.0)
	ChunkIndex  int     // Current chunk number (0-based)
	TotalChunks int     // Total number of chunks
	SessionID   string  // Upload session ID for multi-client support
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

// UploadState represents the current state of an upload
type UploadState int

const (
	StateRunning UploadState = iota
	StatePaused
	StateCancelled
)

// UploadStatus represents the status of an upload session
type UploadStatus string

const (
	StatusQueued    UploadStatus = "queued"
	StatusRunning   UploadStatus = "running"
	StatusPaused    UploadStatus = "paused"
	StatusCompleted UploadStatus = "completed"
	StatusFailed    UploadStatus = "failed"
	StatusCancelled UploadStatus = "cancelled"
)

// EventInfo contains information about upload events
type EventInfo struct {
	Event     UploadEvent // Type of event
	Filename  string      // Name of the file
	Path      string      // Remote path
	Message   string      // Optional message
	Error     error       // Error if applicable
	SessionID string      // Upload session ID for multi-client support
}

// Config holds options for upload operations and provides extensive customization
// for upload behavior, performance optimization, and event handling.
//
// Use DefaultConfig() to get sensible defaults, then customize as needed:
//
//	config := godav.DefaultConfig()
//	config.Verbose = true
//	config.MaxRetries = 5
//	config.ProgressFunc = func(info ProgressInfo) {
//		fmt.Printf("Progress: %.1f%%\n", info.Percentage)
//	}
type Config struct {
	// ChunkSize specifies the size of each chunk in bytes (default 10MB).
	// Larger chunks reduce the number of requests but use more memory.
	// Minimum: 1KB, Maximum: 1GB
	ChunkSize int64

	// SkipExisting when true, skips files that already exist with the same size.
	// This provides efficient synchronization by avoiding unnecessary uploads.
	SkipExisting bool

	// Verbose enables detailed logging of upload operations, including
	// chunk progress, directory creation, and retry attempts.
	Verbose bool

	// ProgressFunc is called during upload to report detailed progress information.
	// The callback receives ProgressInfo with current progress, percentages,
	// and chunk information. Called after each chunk upload.
	ProgressFunc func(info ProgressInfo)

	// EventFunc is called for various upload lifecycle events such as
	// upload started, chunk uploaded, upload completed, etc.
	// Use this for implementing custom upload monitoring and logging.
	EventFunc func(info EventInfo)

	// MaxRetries specifies the maximum number of retry attempts for failed chunks.
	// Each chunk will be retried up to this many times before giving up.
	// Range: 0-10 (default 3)
	MaxRetries int

	// BufferPool provides memory-efficient buffer reuse for upload operations.
	// When specified, buffers will be reused to reduce garbage collection.
	// Use NewBufferPool() to create a pool with desired size and count.
	BufferPool *BufferPool

	// Controller enables pause/resume/cancel functionality for uploads.
	// When specified, the upload can be controlled programmatically.
	// Use NewUploadController() or NewSimpleUploadController() to create.
	Controller *UploadController

	// CheckpointFunc is called periodically to save upload progress.
	// The callback receives a Checkpoint struct that can be persisted
	// and used later to resume interrupted uploads.
	CheckpointFunc func(cp Checkpoint)

	// ResumeFromCheckpoint when specified, resumes an upload from the
	// given checkpoint instead of starting a new upload.
	// Load checkpoints using LoadCheckpoint().
	ResumeFromCheckpoint *Checkpoint
}
