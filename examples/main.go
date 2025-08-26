package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tlmanz/godav"
)

func main() {
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()
	config.Verbose = true

	// Performance optimizations
	config.MaxRetries = 5                                        // Retry failed chunks
	config.BufferPool = godav.NewBufferPool(config.ChunkSize, 8) // Reuse buffers

	// Show upload progress
	config.ProgressFunc = func(info godav.ProgressInfo) {
		fmt.Printf("Uploading %s: %.1f%% (chunk %d/%d)\n",
			info.Filename, info.Percentage, info.ChunkIndex+1, info.TotalChunks)
	}

	// Handle upload lifecycle events
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

	// Upload with context and timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Upload a single file with context support
	err := client.UploadFileWithContext(ctx, "/path/to/local/file.txt", "remote/path/file.txt", config)
	if err != nil {
		// Handle different error types
		if err == context.DeadlineExceeded {
			fmt.Fprintf(os.Stderr, "Upload timed out\n")
		} else if err == context.Canceled {
			fmt.Fprintf(os.Stderr, "Upload was cancelled\n")
		} else {
			var uploadErr *godav.UploadError
			if errors.As(err, &uploadErr) {
				fmt.Fprintf(os.Stderr, "Upload failed: %s (retries: %d)\n", uploadErr.Op, uploadErr.Retries)
			} else {
				fmt.Fprintf(os.Stderr, "File upload error: %v\n", err)
			}
		}
		return
	}

	// Upload a directory recursively
	err = client.UploadDir("/path/to/local/dir", "remote/path/dir", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Directory upload error: %v\n", err)
	}
}
