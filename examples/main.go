package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tlmanz/godav"
)

func basicUploadExample() {
	fmt.Println("=== Basic Upload Example ===")
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
		case godav.EventUploadComplete:
			fmt.Printf("✅ Upload complete: %s\n", info.Filename)
		case godav.EventUploadPaused:
			fmt.Printf("⏸️ Upload paused: %s\n", info.Filename)
		case godav.EventUploadResumed:
			fmt.Printf("▶️ Upload resumed: %s\n", info.Filename)
		case godav.EventUploadFailed:
			fmt.Printf("❌ Upload failed: %s\n", info.Message)
		}
	}

	// Basic upload
	if err := client.UploadFile("test.txt", "/remote/path/test.txt", config); err != nil {
		var uploadErr *godav.UploadError
		if errors.As(err, &uploadErr) {
			fmt.Printf("Upload error: %v\n", uploadErr.Err)
		} else {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func pauseResumeExample() {
	fmt.Println("\n=== Pause/Resume Upload Example ===")
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()
	config.Verbose = true

	// Enable pause/resume functionality
	controller := godav.NewUploadController()
	config.Controller = controller

	// Setup checkpoint saving
	checkpointFile := "/tmp/upload_checkpoint.json"
	config.CheckpointFunc = func(checkpoint godav.Checkpoint) {
		fmt.Printf("💾 Saving checkpoint: %d/%d chunks uploaded\n",
			checkpoint.ChunksUploaded, checkpoint.TotalChunks)
		if err := godav.SaveCheckpoint(checkpoint, checkpointFile); err != nil {
			fmt.Printf("Error saving checkpoint: %v\n", err)
		}
	}

	// Show upload progress
	config.ProgressFunc = func(info godav.ProgressInfo) {
		fmt.Printf("Uploading %s: %.1f%% (chunk %d/%d)\n",
			info.Filename, info.Percentage, info.ChunkIndex+1, info.TotalChunks)
	}

	// Handle upload events
	config.EventFunc = func(info godav.EventInfo) {
		switch info.Event {
		case godav.EventUploadStarted:
			fmt.Printf("🚀 Started uploading: %s\n", info.Filename)
		case godav.EventUploadPaused:
			fmt.Printf("⏸️ Upload paused: %s\n", info.Filename)
		case godav.EventUploadResumed:
			fmt.Printf("▶️ Upload resumed: %s\n", info.Filename)
		case godav.EventUploadComplete:
			fmt.Printf("✅ Upload complete: %s\n", info.Filename)
		}
	}

	// Check for existing checkpoint
	if checkpoint, err := godav.LoadCheckpoint(checkpointFile); err == nil {
		fmt.Printf("📄 Found checkpoint, resuming upload...\n")
		if err := client.ResumeUpload(*checkpoint, config); err != nil {
			fmt.Printf("Error resuming upload: %v\n", err)
		}
	} else {
		// Start new upload
		controller, err := client.UploadFileResumable("large_file.zip", "/remote/path/large_file.zip", config)
		if err != nil {
			fmt.Printf("Error starting resumable upload: %v\n", err)
			return
		}

		// Simulate pausing after 2 seconds
		go func() {
			time.Sleep(2 * time.Second)
			fmt.Println("⏸️ Pausing upload...")
			controller.Pause()

			// Resume after 3 seconds
			time.Sleep(3 * time.Second)
			fmt.Println("▶️ Resuming upload...")
			controller.Resume()
		}()

		// Monitor upload state (simplified for demo)
		time.Sleep(10 * time.Second)
	}
}

func timeoutExample() {
	fmt.Println("\n=== Upload with Timeout Example ===")
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()

	// Upload with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.UploadFileWithContext(ctx, "document.pdf", "/remote/path/document.pdf", config); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Printf("Upload timed out\n")
		} else {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func main() {
	basicUploadExample()
	pauseResumeExample()
	timeoutExample()

	fmt.Println("Done!")
}
