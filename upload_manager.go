// Package godav - Multi-session upload management
//
// This file provides coordination and management for multiple concurrent upload sessions.
// It includes session lifecycle management, global pause/resume functionality,
// and thread-safe coordination between different upload clients.
//
// Features:
//   - Multi-client upload coordination
//   - Session lifecycle management (queued, running, paused, completed, failed, cancelled)
//   - Global pause/resume across all uploads
//   - Thread-safe session state management
//   - Session cleanup and resource management
package godav

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// UploadManager manages multiple concurrent uploads across different clients
type UploadManager struct {
	sessions   map[string]*UploadSession
	globalCtrl *GlobalController
	mu         sync.RWMutex
}

// UploadSession represents a single upload session
type UploadSession struct {
	ID         string
	LocalPath  string
	RemotePath string
	Client     *Client
	Controller *UploadController
	Config     *Config
	Status     UploadStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewUploadManager creates a new upload manager for coordinating multiple uploads.
// The manager provides session lifecycle management, global pause/resume functionality,
// and thread-safe coordination between different upload clients.
//
// Returns a configured UploadManager ready to manage upload sessions.
//
// Example:
//
//	manager := godav.NewUploadManager()
//	session, err := manager.AddUploadSession(localPath, remotePath, client, config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	err = manager.StartUpload(session.ID)
func NewUploadManager() *UploadManager {
	return &UploadManager{
		sessions: make(map[string]*UploadSession),
		globalCtrl: &GlobalController{
			pauseCh:  make(chan struct{}, 1),
			resumeCh: make(chan struct{}, 1),
		},
	}
}

// AddUploadSession adds a new upload session to the manager.
// This creates a new session in "queued" status that can be started later.
// Each session gets a unique ID and its own upload controller.
//
// Parameters:
//   - localPath: Local file path to upload
//   - remotePath: Remote destination path
//   - client: Configured godav client for uploads
//   - config: Upload configuration (nil uses DefaultConfig)
//
// Returns the created session and any error during session creation.
//
// Example:
//
//	session, err := manager.AddUploadSession("/local/file.txt", "remote/file.txt", client, config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Created session: %s\n", session.ID)
func (um *UploadManager) AddUploadSession(localPath, remotePath string, client *Client) (*UploadSession, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	sessionID := fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), filepath.Base(localPath))

	controller := NewUploadController(sessionID, um)
	if client.config == nil {
		client.config = DefaultConfig()
	}
	client.config.Controller = controller

	session := &UploadSession{
		ID:         sessionID,
		LocalPath:  localPath,
		RemotePath: remotePath,
		Client:     client,
		Controller: controller,
		Status:     StatusQueued,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	um.sessions[sessionID] = session
	return session, nil
}

// StartUpload starts an upload session
func (um *UploadManager) StartUpload(sessionID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Status != StatusQueued && session.Status != StatusPaused {
		return fmt.Errorf("session %s cannot be started (current status: %s)", sessionID, session.Status)
	}

	session.Status = StatusRunning
	session.UpdatedAt = time.Now()

	// Start upload in goroutine
	go func() {
		err := session.Client.UploadFile(session.LocalPath, session.RemotePath)
		um.mu.Lock()
		if err != nil {
			session.Status = StatusFailed
		} else {
			session.Status = StatusCompleted
		}
		session.UpdatedAt = time.Now()
		um.mu.Unlock()
	}()

	return nil
}

// PauseUpload pauses a specific upload session
func (um *UploadManager) PauseUpload(sessionID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Status != StatusRunning {
		return fmt.Errorf("session %s is not running", sessionID)
	}

	session.Controller.Pause()
	session.Status = StatusPaused
	session.UpdatedAt = time.Now()
	return nil
}

// ResumeUpload resumes a specific upload session
func (um *UploadManager) ResumeUpload(sessionID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Status != StatusPaused {
		return fmt.Errorf("session %s is not paused", sessionID)
	}

	session.Controller.Resume()
	session.Status = StatusRunning
	session.UpdatedAt = time.Now()
	return nil
}

// PauseAllUploads pauses all running uploads
func (um *UploadManager) PauseAllUploads() {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.globalCtrl.mu.Lock()
	um.globalCtrl.globalPaused = true
	um.globalCtrl.mu.Unlock()

	for _, session := range um.sessions {
		if session.Status == StatusRunning {
			session.Controller.Pause()
			session.Status = StatusPaused
			session.UpdatedAt = time.Now()
		}
	}
}

// ResumeAllUploads resumes all paused uploads
func (um *UploadManager) ResumeAllUploads() {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.globalCtrl.mu.Lock()
	um.globalCtrl.globalPaused = false
	um.globalCtrl.mu.Unlock()

	for _, session := range um.sessions {
		if session.Status == StatusPaused {
			session.Controller.Resume()
			session.Status = StatusRunning
			session.UpdatedAt = time.Now()
		}
	}
}

// GetUploadSessions returns all upload sessions
func (um *UploadManager) GetUploadSessions() map[string]*UploadSession {
	um.mu.RLock()
	defer um.mu.RUnlock()

	// Return a copy to avoid race conditions
	sessions := make(map[string]*UploadSession)
	for id, session := range um.sessions {
		// Create a copy of the session
		sessionCopy := *session
		sessions[id] = &sessionCopy
	}
	return sessions
}

// GetUploadSession returns a specific upload session
func (um *UploadManager) GetUploadSession(sessionID string) (*UploadSession, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	session, exists := um.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Return a copy
	sessionCopy := *session
	return &sessionCopy, nil
}

// RemoveUploadSession removes a completed or failed upload session
func (um *UploadManager) RemoveUploadSession(sessionID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Status == StatusRunning {
		return fmt.Errorf("cannot remove running session %s", sessionID)
	}

	delete(um.sessions, sessionID)
	return nil
}

// IsGloballyPaused returns whether all uploads are globally paused
func (um *UploadManager) IsGloballyPaused() bool {
	um.globalCtrl.mu.RLock()
	defer um.globalCtrl.mu.RUnlock()
	return um.globalCtrl.globalPaused
}
