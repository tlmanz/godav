// Package godav - Upload checkpoint and resumption functionality
//
// This file provides upload resumption capabilities through checkpoint persistence.
// Checkpoints contain all necessary information to resume an interrupted upload,
// including upload progress, chunk information, and configuration settings.
//
// Features:
//   - JSON-based checkpoint serialization
//   - File-based checkpoint persistence
//   - Resume upload from saved checkpoints
//   - Configuration restoration from checkpoints
package godav

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Checkpoint represents a resumable upload checkpoint containing all necessary
// information to resume an interrupted upload. It includes upload progress,
// file information, and essential configuration settings.
//
// Checkpoints are typically saved periodically during upload and can be
// persisted to files, databases, or other storage systems for later resumption.
type Checkpoint struct {
	LocalPath      string    `json:"local_path"`      // Original local file path
	RemotePath     string    `json:"remote_path"`     // Target remote path
	UploadID       string    `json:"upload_id"`       // Unique upload session ID
	FileSize       int64     `json:"file_size"`       // Total file size in bytes
	ChunkSize      int64     `json:"chunk_size"`      // Size of each chunk
	BytesUploaded  int64     `json:"bytes_uploaded"`  // Bytes successfully uploaded
	ChunksUploaded int       `json:"chunks_uploaded"` // Number of chunks uploaded
	TotalChunks    int       `json:"total_chunks"`    // Total number of chunks
	Timestamp      time.Time `json:"timestamp"`       // When checkpoint was created
	// Essential config values (function pointers cannot be serialized)
	ConfigChunkSize    int64 `json:"config_chunk_size"`    // Original chunk size setting
	ConfigSkipExisting bool  `json:"config_skip_existing"` // Skip existing files setting
	ConfigMaxRetries   int   `json:"config_max_retries"`   // Max retry attempts setting
}

// SaveCheckpoint saves a checkpoint to a file in JSON format.
// The checkpoint can later be loaded and used to resume an interrupted upload.
//
// Parameters:
//   - checkpoint: The checkpoint data to save
//   - filePath: Path where the checkpoint file will be created
//
// Returns an error if the checkpoint cannot be serialized or written to file.
//
// Example:
//
//	checkpoint := Checkpoint{...}
//	err := godav.SaveCheckpoint(checkpoint, "/tmp/upload.checkpoint")
//	if err != nil {
//		log.Printf("Failed to save checkpoint: %v", err)
//	}
func SaveCheckpoint(checkpoint Checkpoint, filePath string) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	return os.WriteFile(filePath, data, 0600)
}

// LoadCheckpoint loads a checkpoint from a JSON file.
// The loaded checkpoint can be used to resume an interrupted upload.
//
// Parameters:
//   - filePath: Path to the checkpoint file to load
//
// Returns the loaded checkpoint and any error during loading or parsing.
//
// Example:
//
//	checkpoint, err := godav.LoadCheckpoint("/tmp/upload.checkpoint")
//	if err != nil {
//		log.Printf("No checkpoint found: %v", err)
//		return
//	}
//	err = client.ResumeUpload(*checkpoint, config)
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

// ResumeUpload resumes an upload from a checkpoint.
// It creates an isolated config copy for the resume operation so that
// c.config is never mutated and no stale ResumeFromCheckpoint lingers.
func (c *Client) ResumeUpload(checkpoint Checkpoint) error {
	cfg := validateConfigWith(c.config)
	cfg.ChunkSize = checkpoint.ConfigChunkSize
	cfg.SkipExisting = checkpoint.ConfigSkipExisting
	cfg.MaxRetries = checkpoint.ConfigMaxRetries
	cfg.ResumeFromCheckpoint = &checkpoint

	// Re-validate after overriding checkpoint values.
	cfg = validateConfigWith(cfg)

	return c.uploadFileCore(context.Background(), checkpoint.LocalPath, checkpoint.RemotePath, cfg)
}
