// Package godav - Chunked upload implementation
//
// This file contains the core chunked upload logic for Nextcloud WebDAV,
// implementing the protocol for large file uploads with pause/resume support,
// retry logic, and progress tracking.
//
// The chunked upload protocol follows these steps:
//  1. MKCOL /uploads/<user>/<upload-id>
//  2. PUT /uploads/<user>/<upload-id>/<offset> for each chunk
//  3. MOVE /uploads/<user>/<upload-id>/.file -> /files/<user>/<dst>
package godav

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// uploadChunked performs the Nextcloud chunked upload protocol:
// 1) MKCOL /uploads/<user>/<upload-id>
// 2) PUT /uploads/<user>/<upload-id>/<offset> for each chunk
// 3) MOVE /uploads/<user>/<upload-id>/.file -> /files/<user>/<dst>
func (c *Client) uploadChunked(ctx context.Context, localPath, finalPath string, cfg *Config) error {
	// Cache filename to avoid repeated path.Base calls
	filename := filepath.Base(localPath)

	// Check if resuming from checkpoint
	var uploadID string
	var uploadBase string
	var startOffset int64
	var sent int64
	var chunkIndex int

	if cfg.ResumeFromCheckpoint != nil {
		// Resume from checkpoint
		uploadID = cfg.ResumeFromCheckpoint.UploadID
		uploadBase = c.pathJoinMany("uploads", c.username, uploadID)
		sent = cfg.ResumeFromCheckpoint.BytesUploaded
		chunkIndex = cfg.ResumeFromCheckpoint.ChunksUploaded
		startOffset = int64(chunkIndex) * cfg.ChunkSize

		c.emitEvent(cfg, EventUploadResumed, filename, finalPath,
			fmt.Sprintf("Resuming from chunk %d", chunkIndex), nil)
	} else {
		// New upload
		uploadID = c.newUploadID()
		uploadBase = c.pathJoinMany("uploads", c.username, uploadID)

		// Respect context before network call
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

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

	chunkSize := cfg.ChunkSize

	// Use buffer pool if available, otherwise allocate
	var buf []byte
	if cfg.BufferPool != nil {
		buf = cfg.BufferPool.Get()
		defer cfg.BufferPool.Put(buf)
		// Resize buffer if needed
		if int64(len(buf)) != chunkSize {
			buf = make([]byte, chunkSize)
		}
	} else {
		buf = make([]byte, chunkSize)
	}

	totalChunks := calculateChunks(total, chunkSize)

	for offset := startOffset; offset < total; offset += chunkSize {
		// Early cancellation check each iteration
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Check for pause/resume/cancel
		if cfg.Controller != nil {
			switch cfg.Controller.State() {
			case StatePaused:
				c.emitEvent(cfg, EventUploadPaused, filename, finalPath, "Upload paused", nil)

				// Save checkpoint
				if cfg.CheckpointFunc != nil {
					checkpoint := Checkpoint{
						LocalPath:          localPath,
						RemotePath:         finalPath,
						UploadID:           uploadID,
						FileSize:           total,
						ChunkSize:          chunkSize,
						BytesUploaded:      sent,
						ChunksUploaded:     chunkIndex,
						TotalChunks:        totalChunks,
						Timestamp:          time.Now(),
						ConfigChunkSize:    cfg.ChunkSize,
						ConfigSkipExisting: cfg.SkipExisting,
						ConfigMaxRetries:   cfg.MaxRetries,
					}
					cfg.CheckpointFunc(checkpoint)
				}

				// Wait for resume or cancel or context done
				select {
				case <-cfg.Controller.resumeCh:
					c.emitEvent(cfg, EventUploadResumed, filename, finalPath, "Upload resumed", nil)
				case <-ctx.Done():
					return ctx.Err()
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
		if rerr != nil {
			// io.ErrUnexpectedEOF means the file shrank during upload — treat as an error
			// to avoid uploading a corrupt (truncated) chunk silently.
			return fmt.Errorf("read chunk at %d: %w", offset, rerr)
		}

		// Retry logic for chunk upload
		var uploadErr error
		for retry := 0; retry <= cfg.MaxRetries; retry++ {
			// Check cancellation before each network write
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			chunkPath := c.pathJoin(uploadBase, strconv.FormatInt(offset, 10))
			uploadErr = c.Write(chunkPath, buf[:n], 0o644)
			if uploadErr == nil {
				break // Success
			}

			if retry < cfg.MaxRetries {
				if cfg.Verbose {
					log.Printf("chunk upload retry %d/%d for %s: %v", retry+1, cfg.MaxRetries, chunkPath, uploadErr)
				}
			}
		}

		if uploadErr != nil {
			return &UploadError{
				Op:      "chunk upload",
				Path:    c.pathJoin(uploadBase, strconv.FormatInt(offset, 10)),
				Err:     uploadErr,
				Retries: cfg.MaxRetries,
			}
		}

		sent += int64(n)
		chunkIndex++

		// Emit chunk uploaded event
		c.emitEvent(cfg, EventChunkUploaded, filename, finalPath,
			fmt.Sprintf("Chunk %d/%d uploaded", chunkIndex, totalChunks), nil)

		// Call progress callbacks if provided
		if cfg.ProgressFunc != nil {
			sessionID := ""
			if cfg.Controller != nil {
				sessionID = cfg.Controller.sessionID
			}

			percentage := float64(sent) / float64(total) * 100.0
			info := ProgressInfo{
				Filename:    filename,
				Current:     sent,
				Total:       total,
				Percentage:  percentage,
				ChunkIndex:  chunkIndex - 1, // 0-based
				TotalChunks: totalChunks,
				SessionID:   sessionID,
			}
			cfg.ProgressFunc(info)
		}

		if cfg.Verbose {
			percentage := float64(sent) / float64(total) * 100.0
			chunkPath := c.pathJoin(uploadBase, strconv.FormatInt(offset, 10))
			log.Printf("chunk %s: +%d bytes (%d/%d, %.1f%%)", chunkPath, n, sent, total, percentage)
		}

		// Save checkpoint periodically (every 10 chunks)
		if cfg.CheckpointFunc != nil && chunkIndex%10 == 0 {
			checkpoint := Checkpoint{
				LocalPath:          localPath,
				RemotePath:         finalPath,
				UploadID:           uploadID,
				FileSize:           total,
				ChunkSize:          chunkSize,
				BytesUploaded:      sent,
				ChunksUploaded:     chunkIndex,
				TotalChunks:        totalChunks,
				Timestamp:          time.Now(),
				ConfigChunkSize:    cfg.ChunkSize,
				ConfigSkipExisting: cfg.SkipExisting,
				ConfigMaxRetries:   cfg.MaxRetries,
			}
			cfg.CheckpointFunc(checkpoint)
		}
	}

	// All chunks uploaded
	c.emitEvent(cfg, EventChunksComplete, filename, finalPath, "All chunks uploaded", nil)

	// Finalize: MOVE to final destination
	c.emitEvent(cfg, EventMoveStarted, filename, finalPath, "Starting final move operation", nil)
	// guard header mutation and rename in case same underlying client is shared
	c.hdrMu.Lock()
	c.SetHeader("OC-Total-Length", strconv.FormatInt(total, 10))
	defer func() {
		c.SetHeader("OC-Total-Length", "")
		c.hdrMu.Unlock()
	}()

	src := c.pathJoin(uploadBase, ".file")
	// Check context before final MOVE
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := c.Rename(src, finalPath, true); err != nil {
		_ = c.RemoveAll(uploadBase)
		return fmt.Errorf("finalize move %s -> %s: %w", src, finalPath, err)
	}

	c.emitEvent(cfg, EventMoveComplete, filename, finalPath, "Move operation completed", nil)

	// Cleanup
	_ = c.RemoveAll(uploadBase)

	if cfg.Verbose {
		log.Printf("Uploaded (chunked): %s", finalPath)
	}

	return nil
}

// UploadFileResumable uploads a file with built-in pause/resume support
func (c *Client) UploadFileResumable(localPath, dstPath string) (*UploadController, error) {
	if c.config == nil {
		c.config = DefaultConfig()
	}

	// Create upload controller if not provided
	if c.config.Controller == nil {
		c.config.Controller = NewSimpleUploadController()
	}

	// Upload in a goroutine to enable pause/resume.
	// uploadFileCore already emits EventUploadFailed on error, so no duplicate emission here.
	go func() {
		_ = c.UploadFile(localPath, dstPath)
	}()

	return c.config.Controller, nil
}
