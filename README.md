[![CI](https://github.com/tlmanz/godav/actions/workflows/ci.yml/badge.svg)](https://github.com/tlmanz/godav/actions/workflows/ci.yml)
[![CodeQL](https://github.com/tlmanz/godav/actions/workflows/codequality.yml/badge.svg)](https://github.com/tlmanz/godav/actions/workflows/codequality.yml)
[![Coverage Status](https://coveralls.io/repos/github/tlmanz/godav/badge.svg)](https://coveralls.io/github/tlmanz/godav)
![Open Issues](https://img.shields.io/github/issues/tlmanz/godav)
[![Go Report Card](https://goreportcard.com/badge/github.com/tlmanz/godav)](https://goreportcard.com/report/github.com/tlmanz/godav)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/tlmanz/godav)

# godav

🚀 A Go library for WebDAV with full support for Nextcloud chunked uploads and advanced file operations.

## Architecture

The library is organized into focused modules for better maintainability and clarity:

- **`client.go`** - Core client functionality and basic upload methods
- **`types.go`** - Type definitions, constants, and configuration structures  
- **`chunked_upload.go`** - Chunked upload implementation with retry logic
- **`upload_controller.go`** - Pause/resume/cancel functionality for uploads
- **`upload_manager.go`** - Multi-session upload coordination and management
- **`checkpoint.go`** - Upload resumption and checkpoint persistence
- **`buffer_pool.go`** - Memory-efficient buffer management
- **`utils.go`** - Helper functions and utilities

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

### Basic Upload

```go
package main

import (
	"log"
	"github.com/tlmanz/godav"
)

func main() {
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	cfg := godav.DefaultConfig()
	cfg.Verbose = true

	// Upload a single file (pass config explicitly)
	err := client.UploadFileWithConfig("/path/to/local/file.txt", "remote/path/file.txt", cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Upload a directory recursively (uses the client's internal config)
	// For now, set simple flags via methods like SetVerbose; per-call config is not supported for directories.
	client.SetVerbose(true)
	err = client.UploadDir("/path/to/local/dir", "remote/path/dir")
	if err != nil {
		log.Fatal(err)
	}
}
```

### Advanced Usage with All Features

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tlmanz/godav"
)

func main() {
	// Create client
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	
	// Configure advanced upload settings (per-call config)
	cfg := godav.DefaultConfig()
	cfg.Verbose = true
	cfg.MaxRetries = 5
	cfg.BufferPool = godav.NewBufferPool(cfg.ChunkSize, 8) // 8 reusable buffers

	// Setup progress tracking
	cfg.ProgressFunc = func(info godav.ProgressInfo) {
		fmt.Printf("[%s] Progress: %.1f%% (chunk %d/%d)\n", 
			info.SessionID, info.Percentage, info.ChunkIndex+1, info.TotalChunks)
	}
	
	// Setup event tracking
	cfg.EventFunc = func(info godav.EventInfo) {
		switch info.Event {
		case godav.EventUploadStarted:
			fmt.Printf("🚀 Started: %s\n", info.Filename)
		case godav.EventUploadComplete:
			fmt.Printf("✅ Completed: %s\n", info.Filename)
		case godav.EventUploadFailed:
			fmt.Printf("❌ Failed: %s - %v\n", info.Filename, info.Error)
		case godav.EventUploadPaused:
			fmt.Printf("⏸️ Paused: %s\n", info.Filename)
		case godav.EventUploadResumed:
			fmt.Printf("▶️ Resumed: %s\n", info.Filename)
		}
	}
	
	// Setup checkpoint saving for resume capability
	checkpointFile := "/tmp/upload.checkpoint"
	cfg.CheckpointFunc = func(checkpoint godav.Checkpoint) {
		fmt.Printf("💾 Saving checkpoint: %d/%d chunks\n", 
			checkpoint.ChunksUploaded, checkpoint.TotalChunks)
		if err := godav.SaveCheckpoint(checkpoint, checkpointFile); err != nil {
			log.Printf("Failed to save checkpoint: %v", err)
		}
	}
	
	// Check for existing checkpoint
	if checkpoint, err := godav.LoadCheckpoint(checkpointFile); err == nil {
		fmt.Printf("📄 Found checkpoint, resuming from %d/%d bytes\n", 
			checkpoint.BytesUploaded, checkpoint.FileSize)
		cfg.ResumeFromCheckpoint = checkpoint
	}
	
	// Add upload session to manager
	localPath := "/path/to/large/file.mkv"
	remotePath := "Movies/movie.mkv"
	
	// Start an upload with context and full control using the per-call config
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
    
	go func() {
		if err := client.UploadFileWithContextWithConfig(ctx, localPath, remotePath, cfg); err != nil {
			log.Printf("upload error: %v", err)
		}
	}()

	// Optional: graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigCh
		fmt.Println("\n🛑 Shutdown signal received, pausing uploads...")
		// In a full application, trigger your controller to pause and persist a final checkpoint
		// (see Pause/Resume section below)
		
		os.Exit(0)
	}()
	// ... your app continues while the upload runs in background
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
// Create a controller and pass it via config
controller := godav.NewUploadController("session-1", nil)
cfg := godav.DefaultConfig()
cfg.Controller = controller
cfg.CheckpointFunc = func(checkpoint godav.Checkpoint) {
	_ = godav.SaveCheckpoint(checkpoint, "/tmp/upload_checkpoint.json")
}

// Start resumable upload with config
go client.UploadFileWithConfig(localPath, remotePath, cfg)

// Control programmatically
controller.Pause()
controller.Resume()
controller.Cancel()

// Resume from a saved checkpoint (two options)
if cp, err := godav.LoadCheckpoint("/tmp/upload_checkpoint.json"); err == nil {
	// A) One-call quick resume
	_ = client.ResumeUpload(*cp)

	// B) Full control resume with callbacks
	cfg := godav.DefaultConfig()
	cfg.CheckpointFunc = func(c godav.Checkpoint) {
		_ = godav.SaveCheckpoint(c, "/tmp/upload_checkpoint.json")
	}
	cfg.ResumeFromCheckpoint = cp
	_ = client.UploadFileWithConfig(cp.LocalPath, cp.RemotePath, cfg)
}
```

### Performance Configuration

For high-performance uploads, configure buffer pooling and retry logic:

```go
cfg := godav.DefaultConfig()
cfg.MaxRetries = 5                                    // Retry failed chunks up to 5 times
cfg.BufferPool = godav.NewBufferPool(cfg.ChunkSize, 8) // Pool of 8 reusable buffers
```

### Context Support

Use context for cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
defer cancel()

err := client.UploadFileWithContextWithConfig(ctx, localPath, remotePath, cfg)
if err != nil {
    if err == context.DeadlineExceeded {
        fmt.Println("Upload timed out")
    } else if err == context.Canceled {
        fmt.Println("Upload was cancelled")
    }
}
```
Note: Context is honored throughout the upload lifecycle (MKCOL, per-chunk PUTs, and the final MOVE), so cancellations and timeouts interrupt promptly.

### Progress Tracking

Show upload progress with detailed information:

```go
cfg.ProgressFunc = func(info godav.ProgressInfo) {
	fmt.Printf("Uploading %s: %.1f%% (chunk %d/%d)\n", 
		info.Filename, info.Percentage, info.ChunkIndex+1, info.TotalChunks)
}
```

### Upload Lifecycle Events

Track different stages of the upload process:

```go
cfg.EventFunc = func(info godav.EventInfo) {
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
cfg := godav.DefaultConfig()
err := client.UploadFileWithConfig(localPath, remotePath, cfg)
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

## Library Design

The godav library follows a modular design pattern where functionality is separated into focused files:

### Core Components

- **Client (`client.go`)**: Main client interface with basic upload operations
- **Types (`types.go`)**: Centralized type definitions and configuration structures
- **Chunked Upload (`chunked_upload.go`)**: Core upload algorithm implementation

### Advanced Features

- **Upload Controller (`upload_controller.go`)**: Individual upload state management
- **Upload Manager (`upload_manager.go`)**: Multi-session coordination
- **Checkpoint (`checkpoint.go`)**: Resume functionality and persistence
- **Buffer Pool (`buffer_pool.go`)**: Memory optimization utilities
- **Utils (`utils.go`)**: Helper functions and utilities

This modular approach provides:
- **Maintainability**: Each module has a single, clear responsibility
- **Testability**: Components can be tested in isolation
- **Extensibility**: New features can be added without affecting existing code
- **Readability**: Smaller, focused files are easier to understand

## License

This project is licensed under the MIT License.
