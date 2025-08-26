package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	if err := client.UploadFile("/path/to/local/file.txt", "remote/path/file.txt", config); err != nil {
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
	client := godav.NewClient("https://storage.slt.lk/drive/remote.php/dav/", "4a029cf7-4193-4907-bf6c-5371f4b74e9f", "Yaz59-7fCp3-cBMk7-rTygp-ybsgc")
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
		case godav.EventUploadFailed:
			fmt.Printf("❌ Upload failed: %s - %v\n", info.Filename, info.Error)
		}
	}

	// Check for existing checkpoint first
	if checkpoint, err := godav.LoadCheckpoint(checkpointFile); err == nil {
		fmt.Printf("📄 Found checkpoint, resuming upload from %d/%d bytes...\n",
			checkpoint.BytesUploaded, checkpoint.FileSize)
		if err := client.ResumeUpload(*checkpoint, config); err != nil {
			fmt.Printf("Error resuming upload: %v\n", err)
		} else {
			// Clean up checkpoint on successful resume
			os.Remove(checkpointFile)
		}
		return
	}

	// Start new upload with pause/resume capability
	go func() {
		// Simulate pausing after 2 seconds
		time.Sleep(2 * time.Second)
		fmt.Println("🛑 Pausing upload...")
		controller.Pause()

		// Resume after 3 seconds
		time.Sleep(3 * time.Second)
		fmt.Println("▶️ Resuming upload...")
		controller.Resume()
	}()

	// Start the upload
	if err := client.UploadFile("/home/tharuka/Desktop/Personal/gowebdav/test/test.mkv", "Movies/movie.mkv", config); err != nil {
		fmt.Printf("Upload failed: %v\n", err)
		fmt.Println("📄 Checkpoint saved. You can resume this upload later by running the example again.")
	} else {
		// Clean up checkpoint file on success
		os.Remove(checkpointFile)
		fmt.Println("🧹 Checkpoint file cleaned up")
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
	fmt.Println("🚀 godav Examples")
	fmt.Println("================")

	// Uncomment the example you want to run:

	// Example 1: Basic upload with progress and events
	// basicUploadExample()

	// Example 2: Pause/Resume upload with checkpoints
	pauseResumeExample()

	// Example 3: Upload with timeout
	// timeoutExample()

	fmt.Println("\n✅ Example completed!")
}
