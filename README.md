[![CI](https://github.com/tlmanz/godav/actions/workflows/ci.yml/badge.svg)](https://github.com/tlmanz/godav/actions/workflows/ci.yml)
[![CodeQL](https://github.com/tlmanz/godav/actions/workflows/codequality.yml/badge.svg)](https://github.com/tlmanz/godav/actions/workflows/codequality.yml)
[![Coverage Status](https://coveralls.io/repos/github/tlmanz/godav/badge.svg)](https://coveralls.io/github/tlmanz/godav)
![Open Issues](https://img.shields.io/github/issues/tlmanz/godav)
[![Go Report Card](https://goreportcard.com/badge/github.com/tlmanz/godav)](https://goreportcard.com/report/github.com/tlmanz/godav)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/tlmanz/godav)

# godav

🚀 A Go library for WebDAV with full support for Nextcloud chunked uploads and advanced file operations.

## Features

- High-level client for Nextcloud WebDAV
- Chunked uploads (bypass proxy body-size limits)
- Recursive directory uploads
- Progress reporting and verbose logging
- Skips files that already exist with the same size
- **Performance optimizations:**
  - Buffer pooling to reduce memory allocations
  - Automatic retry logic for failed chunks
  - Context support for cancellation
  - Input validation and sanitization
  - Efficient error handling with custom error types
- **Pause/Resume functionality:**
  - Pause and resume uploads at any time
  - Automatic checkpoint saving and loading
  - Resume from interruptions or failures
  - Graceful handling of network disconnections

## Installation

```sh
go get github.com/tlmanz/godav
```

## Usage

```go
package main

import (
	"github.com/tlmanz/godav"
)

func main() {
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()
	config.Verbose = true

	// Upload a single file
	err := client.UploadFile("/path/to/local/file.txt", "remote/path/file.txt", config)
	if err != nil {
		panic(err)
	}

	// Upload a directory recursively
	err = client.UploadDir("/path/to/local/dir", "remote/path/dir", config)
	if err != nil {
		panic(err)
	}
}
```

## Configuration

You can customize upload behavior using the `Config` struct:

```go
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
```

### Pause/Resume Functionality

Enable pause and resume for large file uploads:

```go
// Create an upload controller
controller := godav.NewUploadController()
config.Controller = controller

// Setup checkpoint saving
config.CheckpointFunc = func(checkpoint godav.Checkpoint) {
    // Save checkpoint to file, database, etc.
    godav.SaveCheckpoint(checkpoint, "/tmp/upload_checkpoint.json")
}

// Start resumable upload
controller, err := client.UploadFileResumable(localPath, remotePath, config)

// Control upload programmatically
controller.Pause()  // Pause the upload
controller.Resume() // Resume the upload
controller.Cancel() // Cancel the upload

// Resume from a saved checkpoint
checkpoint, err := godav.LoadCheckpoint("/tmp/upload_checkpoint.json")
if err == nil {
    err = client.ResumeUpload(*checkpoint, config)
}
```

### Performance Configuration

For high-performance uploads, configure buffer pooling and retry logic:

```go
config := godav.DefaultConfig()
config.MaxRetries = 5                                    // Retry failed chunks up to 5 times
config.BufferPool = godav.NewBufferPool(config.ChunkSize, 8) // Pool of 8 reusable buffers
```

### Context Support

Use context for cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
defer cancel()

err := client.UploadFileWithContext(ctx, localPath, remotePath, config)
if err != nil {
    if err == context.DeadlineExceeded {
        fmt.Println("Upload timed out")
    } else if err == context.Canceled {
        fmt.Println("Upload was cancelled")
    }
}
```

### Progress Tracking

Show upload progress with detailed information:

```go
config.ProgressFunc = func(info godav.ProgressInfo) {
	fmt.Printf("Uploading %s: %.1f%% (chunk %d/%d)\n", 
		info.Filename, info.Percentage, info.ChunkIndex+1, info.TotalChunks)
}
```

### Upload Lifecycle Events

Track different stages of the upload process:

```go
config.EventFunc = func(info godav.EventInfo) {
	switch info.Event {
	case godav.EventUploadStarted:
		fmt.Printf("🚀 Started uploading: %s\n", info.Filename)
	case godav.EventChunkUploaded:
		fmt.Printf("📦 %s\n", info.Message)
	case godav.EventChunksComplete:
		fmt.Printf("✅ All chunks uploaded for: %s\n", info.Filename)
	case godav.EventMoveStarted:
		fmt.Printf("🔄 Moving file to final location: %s\n", info.Filename)
	case godav.EventMoveComplete:
		fmt.Printf("📍 File moved successfully: %s\n", info.Filename)
	case godav.EventUploadComplete:
		fmt.Printf("🎉 Upload completed: %s\n", info.Filename)
	case godav.EventUploadFailed:
		fmt.Printf("❌ Upload failed: %s - %v\n", info.Filename, info.Error)
	case godav.EventUploadSkipped:
		fmt.Printf("⏭️  Skipped: %s - %s\n", info.Filename, info.Message)
	}
}
```

### Available Events

- `EventUploadStarted` - Upload process initiated
- `EventChunkUploaded` - Individual chunk uploaded
- `EventChunksComplete` - All chunks uploaded, before move
- `EventMoveStarted` - Starting final move operation
- `EventMoveComplete` - Move operation completed
- `EventUploadComplete` - Entire upload process finished
- `EventUploadFailed` - Upload failed
- `EventUploadSkipped` - File skipped (already exists)
- `EventUploadPaused` - Upload paused
- `EventUploadResumed` - Upload resumed from checkpoint

### Error Handling

The library provides detailed error information:

```go
err := client.UploadFile(localPath, remotePath, config)
if err != nil {
    var uploadErr *godav.UploadError
    if errors.As(err, &uploadErr) {
        fmt.Printf("Upload failed: %s (retries: %d)\n", uploadErr.Op, uploadErr.Retries)
    }
}
```

## Performance Tips

1. **Use buffer pooling**: Configure `BufferPool` to reuse memory buffers
2. **Optimize chunk size**: Larger chunks = fewer requests, but more memory usage
3. **Set appropriate retries**: Balance reliability vs. performance
4. **Use context**: Implement timeouts and cancellation for better UX
5. **Enable pause/resume**: For large files, use checkpoints to recover from interruptions
6. **Handle signals**: Implement graceful shutdown with checkpoint saving

## License

This project is licensed under the MIT License.
