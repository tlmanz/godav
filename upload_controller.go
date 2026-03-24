// Package godav - Upload control and state management
//
// This file provides pause/resume/cancel functionality for individual uploads
// and global control across multiple upload sessions. It includes thread-safe
// state management and coordination between upload sessions.
package godav

import (
	"fmt"
	"sync"
	"time"
)

// GlobalController provides global pause/resume control across all uploads
type GlobalController struct {
	globalPaused bool
	mu           sync.RWMutex
}

// UploadController provides pause/resume functionality for individual uploads
type UploadController struct {
	sessionID string
	state     UploadState
	stateCh   chan UploadState
	resumeCh  chan struct{}
	manager   *UploadManager
	mu        sync.RWMutex
}

// NewUploadController creates a new upload controller for a specific session
func NewUploadController(sessionID string, manager *UploadManager) *UploadController {
	return &UploadController{
		sessionID: sessionID,
		state:     StateRunning,
		stateCh:   make(chan UploadState, 1),
		resumeCh:  make(chan struct{}, 1),
		manager:   manager,
	}
}

// NewSimpleUploadController creates a basic upload controller (for backward compatibility)
func NewSimpleUploadController() *UploadController {
	return &UploadController{
		sessionID: fmt.Sprintf("session-%d", time.Now().UnixNano()),
		state:     StateRunning,
		stateCh:   make(chan UploadState, 1),
		resumeCh:  make(chan struct{}, 1),
	}
}

// Pause pauses the upload
func (uc *UploadController) Pause() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.state = StatePaused
}

// Resume resumes the upload
func (uc *UploadController) Resume() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
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
	uc.mu.Lock()
	uc.state = StateCancelled
	uc.mu.Unlock()
}

// State returns the current upload state
func (uc *UploadController) State() UploadState {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.state
}
