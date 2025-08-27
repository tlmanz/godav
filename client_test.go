package godav

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// mockClient is a minimal stub for testing purposes.
type mockClient struct {
	Client
	writeCalled bool
}

func (m *mockClient) Write(path string, data []byte, perm os.FileMode) error {
	m.writeCalled = true
	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ChunkSize != 10*1024*1024 {
		t.Errorf("expected default chunk size 10MB, got %d", cfg.ChunkSize)
	}
	if !cfg.SkipExisting {
		t.Error("expected SkipExisting to be true by default")
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")
	if c.username != "user" {
		t.Errorf("expected username 'user', got '%s'", c.username)
	}
}

func TestSetVerbose(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")
	c.SetVerbose(true)
	if !c.config.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestProgressInfo(t *testing.T) {
	info := ProgressInfo{
		Filename:    "test.txt",
		Current:     50,
		Total:       100,
		Percentage:  50.0,
		ChunkIndex:  1,
		TotalChunks: 3,
	}

	if info.Filename != "test.txt" {
		t.Errorf("expected filename 'test.txt', got '%s'", info.Filename)
	}
	if info.Current != 50 {
		t.Errorf("expected current 50, got %d", info.Current)
	}
	if info.Total != 100 {
		t.Errorf("expected total 100, got %d", info.Total)
	}
	if info.Percentage != 50.0 {
		t.Errorf("expected percentage 50.0, got %f", info.Percentage)
	}
	if info.ChunkIndex != 1 {
		t.Errorf("expected chunk index 1, got %d", info.ChunkIndex)
	}
	if info.TotalChunks != 3 {
		t.Errorf("expected total chunks 3, got %d", info.TotalChunks)
	}
}

func TestConfigWithProgressFunc(t *testing.T) {
	progressCalled := false
	var capturedInfo ProgressInfo

	config := &Config{
		ChunkSize:    1024,
		SkipExisting: false,
		Verbose:      true,
		ProgressFunc: func(info ProgressInfo) {
			progressCalled = true
			capturedInfo = info
		},
	}

	// Simulate calling the progress function
	testInfo := ProgressInfo{
		Filename:    "example.txt",
		Current:     512,
		Total:       1024,
		Percentage:  50.0,
		ChunkIndex:  0,
		TotalChunks: 2,
	}

	config.ProgressFunc(testInfo)

	if !progressCalled {
		t.Error("expected progress function to be called")
	}
	if capturedInfo.Filename != "example.txt" {
		t.Errorf("expected captured filename 'example.txt', got '%s'", capturedInfo.Filename)
	}
	if capturedInfo.Percentage != 50.0 {
		t.Errorf("expected captured percentage 50.0, got %f", capturedInfo.Percentage)
	}
}

func TestToFilesPath(t *testing.T) {
	c := NewClient("http://example.com", "testuser", "pass")

	tests := []struct {
		input    string
		expected string
	}{
		{"test.txt", "files/testuser/test.txt"},
		{"/test.txt", "files/testuser/test.txt"},
		{"files/testuser/test.txt", "files/testuser/test.txt"},
		{"/files/testuser/test.txt", "files/testuser/test.txt"},
		{"folder/test.txt", "files/testuser/folder/test.txt"},
	}

	for _, test := range tests {
		result := c.toFilesPath(test.input)
		if result != test.expected {
			t.Errorf("toFilesPath(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestPathJoin(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	tests := []struct {
		a, b     string
		expected string
	}{
		{"", "", ""},
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a/b"},
		{"a/", "b", "a/b"},
		{"a", "/b", "a/b"},
		{"a/", "/b", "a/b"},
	}

	for _, test := range tests {
		result := c.pathJoin(test.a, test.b)
		if result != test.expected {
			t.Errorf("pathJoin(%q, %q) = %q, expected %q", test.a, test.b, result, test.expected)
		}
	}
}

func TestPathJoinMany(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	tests := []struct {
		parts    []string
		expected string
	}{
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a/b"},
		{[]string{"a", "b", "c"}, "a/b/c"},
		{[]string{"/a/", "/b/", "/c/"}, "a/b/c"},
		{[]string{"", "a", "", "b", ""}, "a/b"},
	}

	for _, test := range tests {
		result := c.pathJoinMany(test.parts...)
		if result != test.expected {
			t.Errorf("pathJoinMany(%v) = %q, expected %q", test.parts, result, test.expected)
		}
	}
}

func TestDirOf(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	tests := []struct {
		path     string
		expected string
	}{
		{"", ""},
		{"file.txt", ""},
		{"dir/file.txt", "dir"},
		{"path/to/file.txt", "path/to"},
		{"/absolute/path/file.txt", "/absolute/path"},
	}

	for _, test := range tests {
		result := c.dirOf(test.path)
		if result != test.expected {
			t.Errorf("dirOf(%q) = %q, expected %q", test.path, result, test.expected)
		}
	}
}

func TestIsAlreadyExists(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{fmt.Errorf("file already exists"), true},
		{fmt.Errorf("405 Method Not Allowed"), true},
		{fmt.Errorf("409 Conflict"), true},
		{fmt.Errorf("some other error"), false},
	}

	for _, test := range tests {
		result := c.isAlreadyExists(test.err)
		if result != test.expected {
			t.Errorf("isAlreadyExists(%v) = %t, expected %t", test.err, result, test.expected)
		}
	}
}

func TestNewUploadID(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	id1 := c.newUploadID()
	id2 := c.newUploadID()

	if id1 == id2 {
		t.Error("expected unique upload IDs")
	}

	if !strings.HasPrefix(id1, "web-file-upload-") {
		t.Errorf("expected upload ID to start with 'web-file-upload-', got %q", id1)
	}
}

func TestEventInfo(t *testing.T) {
	info := EventInfo{
		Event:    EventUploadStarted,
		Filename: "test.txt",
		Path:     "remote/test.txt",
		Message:  "Upload started",
		Error:    nil,
	}

	if info.Event != EventUploadStarted {
		t.Errorf("expected event %s, got %s", EventUploadStarted, info.Event)
	}
	if info.Filename != "test.txt" {
		t.Errorf("expected filename 'test.txt', got '%s'", info.Filename)
	}
	if info.Path != "remote/test.txt" {
		t.Errorf("expected path 'remote/test.txt', got '%s'", info.Path)
	}
	if info.Message != "Upload started" {
		t.Errorf("expected message 'Upload started', got '%s'", info.Message)
	}
	if info.Error != nil {
		t.Errorf("expected no error, got %v", info.Error)
	}
}

func TestConfigWithEventFunc(t *testing.T) {
	eventCalled := false
	var capturedEvent EventInfo

	config := &Config{
		ChunkSize:    1024,
		SkipExisting: false,
		Verbose:      true,
		EventFunc: func(info EventInfo) {
			eventCalled = true
			capturedEvent = info
		},
	}

	// Simulate calling the event function
	testEvent := EventInfo{
		Event:    EventChunkUploaded,
		Filename: "example.txt",
		Path:     "remote/example.txt",
		Message:  "Chunk 1/3 uploaded",
		Error:    nil,
	}

	config.EventFunc(testEvent)

	if !eventCalled {
		t.Error("expected event function to be called")
	}
	if capturedEvent.Event != EventChunkUploaded {
		t.Errorf("expected captured event %s, got %s", EventChunkUploaded, capturedEvent.Event)
	}
	if capturedEvent.Filename != "example.txt" {
		t.Errorf("expected captured filename 'example.txt', got '%s'", capturedEvent.Filename)
	}
}

func TestEmitEvent(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	eventCalled := false
	var capturedEvent EventInfo

	c.config.EventFunc = func(info EventInfo) {
		eventCalled = true
		capturedEvent = info
	}

	// Test emitting an event
	c.emitEvent(EventUploadStarted, "test.txt", "remote/test.txt", "Test message", nil)

	if !eventCalled {
		t.Error("expected event to be emitted")
	}
	if capturedEvent.Event != EventUploadStarted {
		t.Errorf("expected event %s, got %s", EventUploadStarted, capturedEvent.Event)
	}
	if capturedEvent.Filename != "test.txt" {
		t.Errorf("expected filename 'test.txt', got '%s'", capturedEvent.Filename)
	}
	if capturedEvent.Message != "Test message" {
		t.Errorf("expected message 'Test message', got '%s'", capturedEvent.Message)
	}
}

func TestEmitEventWithNilConfig(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	c.config.EventFunc = nil

	// This should not panic
	c.emitEvent(EventUploadStarted, "test.txt", "remote/test.txt", "Test message", nil)
}

func TestUploadEvents(t *testing.T) {
	// Test that all event constants are defined
	events := []UploadEvent{
		EventUploadStarted,
		EventChunkUploaded,
		EventChunksComplete,
		EventMoveStarted,
		EventMoveComplete,
		EventUploadComplete,
		EventUploadFailed,
		EventUploadSkipped,
	}

	expectedEvents := []string{
		"upload_started",
		"chunk_uploaded",
		"chunks_complete",
		"move_started",
		"move_complete",
		"upload_complete",
		"upload_failed",
		"upload_skipped",
	}

	for i, event := range events {
		if string(event) != expectedEvents[i] {
			t.Errorf("expected event %s, got %s", expectedEvents[i], string(event))
		}
	}
}

func TestBufferPool(t *testing.T) {
	chunkSize := int64(1024)
	poolSize := 2
	pool := NewBufferPool(chunkSize, poolSize)

	// Test getting a buffer
	buf1 := pool.Get()
	if int64(len(buf1)) != chunkSize {
		t.Errorf("expected buffer size %d, got %d", chunkSize, len(buf1))
	}

	// Test putting buffer back
	pool.Put(buf1)

	// Test getting buffer again (should be reused)
	buf2 := pool.Get()
	if int64(len(buf2)) != chunkSize {
		t.Errorf("expected buffer size %d, got %d", chunkSize, len(buf2))
	}

	// Test wrong size buffer is not pooled
	wrongSizeBuf := make([]byte, 512)
	pool.Put(wrongSizeBuf)
}

func TestValidateConfig(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	tests := []struct {
		name     string
		input    *Config
		expected func(*Config) bool
	}{
		{
			name:  "nil config",
			input: nil,
			expected: func(cfg *Config) bool {
				return cfg.ChunkSize == 10*1024*1024 && cfg.MaxRetries == 3
			},
		},
		{
			name: "negative chunk size",
			input: &Config{
				ChunkSize:  -1,
				MaxRetries: 3,
			},
			expected: func(cfg *Config) bool {
				return cfg.ChunkSize == 10*1024*1024
			},
		},
		{
			name: "too small chunk size",
			input: &Config{
				ChunkSize:  500,
				MaxRetries: 3,
			},
			expected: func(cfg *Config) bool {
				return cfg.ChunkSize == 1024
			},
		},
		{
			name: "too large chunk size",
			input: &Config{
				ChunkSize:  2 * 1024 * 1024 * 1024, // 2GB
				MaxRetries: 3,
			},
			expected: func(cfg *Config) bool {
				return cfg.ChunkSize == 1024*1024*1024 // 1GB max
			},
		},
		{
			name: "negative retries",
			input: &Config{
				ChunkSize:  1024 * 1024,
				MaxRetries: -1,
			},
			expected: func(cfg *Config) bool {
				return cfg.MaxRetries == 0
			},
		},
		{
			name: "too many retries",
			input: &Config{
				ChunkSize:  1024 * 1024,
				MaxRetries: 20,
			},
			expected: func(cfg *Config) bool {
				return cfg.MaxRetries == 10
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Apply test input config (may be nil) before validation
			c.config = test.input
			result := c.validateConfig()
			if !test.expected(result) {
				t.Errorf("validation failed for %s", test.name)
			}
		})
	}
}

func TestCalculateChunks(t *testing.T) {
	tests := []struct {
		fileSize  int64
		chunkSize int64
		expected  int
	}{
		{1024, 512, 2},
		{1000, 512, 2},
		{512, 512, 1},
		{0, 512, 0},
		{1025, 512, 3},
	}

	for _, test := range tests {
		result := calculateChunks(test.fileSize, test.chunkSize)
		if result != test.expected {
			t.Errorf("calculateChunks(%d, %d) = %d, expected %d",
				test.fileSize, test.chunkSize, result, test.expected)
		}
	}
}

func TestUploadError(t *testing.T) {
	baseErr := fmt.Errorf("network timeout")

	// Test without retries
	err1 := &UploadError{
		Op:      "upload",
		Path:    "/test/file.txt",
		Err:     baseErr,
		Retries: 0,
	}

	expected1 := "upload /test/file.txt: network timeout"
	if err1.Error() != expected1 {
		t.Errorf("expected %q, got %q", expected1, err1.Error())
	}

	// Test with retries
	err2 := &UploadError{
		Op:      "upload",
		Path:    "/test/file.txt",
		Err:     baseErr,
		Retries: 3,
	}

	expected2 := "upload /test/file.txt: network timeout (after 3 retries)"
	if err2.Error() != expected2 {
		t.Errorf("expected %q, got %q", expected2, err2.Error())
	}

	// Test unwrap
	if err2.Unwrap() != baseErr {
		t.Error("Unwrap() should return the original error")
	}
}

func TestContextCancellation(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.UploadFileWithContext(ctx, "/nonexistent", "remote")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDefaultConfigWithBufferPool(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BufferPool == nil {
		t.Error("expected default config to have buffer pool")
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries to be 3, got %d", cfg.MaxRetries)
	}
}

func TestUploadController(t *testing.T) {
	manager := NewUploadManager()
	controller := NewUploadController("test-controller", manager)

	// Test initial state
	if controller.State() != StateRunning {
		t.Errorf("expected initial state to be StateRunning, got %v", controller.State())
	}

	// Test pause
	controller.Pause()
	if controller.State() != StatePaused {
		t.Errorf("expected state to be StatePaused after pause, got %v", controller.State())
	}

	// Test resume
	controller.Resume()
	if controller.State() != StateRunning {
		t.Errorf("expected state to be StateRunning after resume, got %v", controller.State())
	}

	// Test cancel
	controller.Cancel()
	if controller.State() != StateCancelled {
		t.Errorf("expected state to be StateCancelled after cancel, got %v", controller.State())
	}
}

func TestCheckpoint(t *testing.T) {
	checkpoint := Checkpoint{
		LocalPath:      "/path/to/local/file.txt",
		RemotePath:     "/path/to/remote/file.txt",
		UploadID:       "test-upload-123",
		FileSize:       1024000,
		ChunkSize:      1024,
		BytesUploaded:  512000,
		ChunksUploaded: 500,
		TotalChunks:    1000,
		Timestamp:      time.Now(),
	}

	// Test checkpoint fields
	if checkpoint.LocalPath != "/path/to/local/file.txt" {
		t.Errorf("unexpected LocalPath: %s", checkpoint.LocalPath)
	}
	if checkpoint.BytesUploaded != 512000 {
		t.Errorf("unexpected BytesUploaded: %d", checkpoint.BytesUploaded)
	}
	if checkpoint.ChunksUploaded != 500 {
		t.Errorf("unexpected ChunksUploaded: %d", checkpoint.ChunksUploaded)
	}
}

func TestSaveAndLoadCheckpoint(t *testing.T) {
	checkpoint := Checkpoint{
		LocalPath:      "/test/file.txt",
		RemotePath:     "remote/file.txt",
		UploadID:       "test-123",
		FileSize:       1000,
		ChunkSize:      100,
		BytesUploaded:  500,
		ChunksUploaded: 5,
		TotalChunks:    10,
		Timestamp:      time.Now().Truncate(time.Second), // Truncate for comparison
	}

	// Create temp file
	tmpFile := "/tmp/test_checkpoint.json"
	defer os.Remove(tmpFile)

	// Save checkpoint
	err := SaveCheckpoint(checkpoint, tmpFile)
	if err != nil {
		t.Fatalf("failed to save checkpoint: %v", err)
	}

	// Load checkpoint
	loaded, err := LoadCheckpoint(tmpFile)
	if err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}

	// Compare
	if loaded.LocalPath != checkpoint.LocalPath {
		t.Errorf("LocalPath mismatch: %s != %s", loaded.LocalPath, checkpoint.LocalPath)
	}
	if loaded.BytesUploaded != checkpoint.BytesUploaded {
		t.Errorf("BytesUploaded mismatch: %d != %d", loaded.BytesUploaded, checkpoint.BytesUploaded)
	}
	if loaded.UploadID != checkpoint.UploadID {
		t.Errorf("UploadID mismatch: %s != %s", loaded.UploadID, checkpoint.UploadID)
	}
}

func TestUploadFileResumable(t *testing.T) {
	c := NewClient("http://example.com", "user", "pass")

	// Test that controller is created
	controller, err := c.UploadFileResumable("/nonexistent", "remote")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if controller == nil {
		t.Error("expected controller to be created")
	}
	if controller.State() != StateRunning {
		t.Errorf("expected initial state to be StateRunning, got %v", controller.State())
	}
}

func TestNewPauseResumeEvents(t *testing.T) {
	events := []UploadEvent{
		EventUploadPaused,
		EventUploadResumed,
	}

	expectedEvents := []string{
		"upload_paused",
		"upload_resumed",
	}

	for i, event := range events {
		if string(event) != expectedEvents[i] {
			t.Errorf("expected event %s, got %s", expectedEvents[i], string(event))
		}
	}
}

// Tests for Multi-Client Upload Management

func TestNewUploadManager(t *testing.T) {
	manager := NewUploadManager()
	if manager == nil {
		t.Fatal("expected NewUploadManager to return a non-nil manager")
	}

	if manager.sessions == nil {
		t.Error("expected sessions map to be initialized")
	}

	if manager.globalCtrl == nil {
		t.Error("expected global controller to be initialized")
	}

	if manager.IsGloballyPaused() {
		t.Error("expected new manager to not be globally paused")
	}
}

func TestUploadManager_AddUploadSession(t *testing.T) {
	manager := NewUploadManager()
	client := NewClient("http://example.com", "user", "pass")

	session, err := manager.AddUploadSession("/local/test.txt", "/remote/test.txt", client)
	if err != nil {
		t.Fatalf("unexpected error adding session: %v", err)
	}

	if session == nil {
		t.Fatal("expected session to be non-nil")
	}

	if session.ID == "" {
		t.Error("expected session to have an ID")
	}

	if session.LocalPath != "/local/test.txt" {
		t.Errorf("expected LocalPath '/local/test.txt', got '%s'", session.LocalPath)
	}

	if session.RemotePath != "/remote/test.txt" {
		t.Errorf("expected RemotePath '/remote/test.txt', got '%s'", session.RemotePath)
	}

	if session.Status != StatusQueued {
		t.Errorf("expected initial status StatusQueued, got %v", session.Status)
	}

	if session.Controller == nil {
		t.Error("expected session to have a controller")
	}

	if session.Controller.sessionID != session.ID {
		t.Error("expected controller sessionID to match session ID")
	}

	// Check that session is stored in manager
	sessions := manager.GetUploadSessions()
	if len(sessions) != 1 {
		t.Errorf("expected 1 session in manager, got %d", len(sessions))
	}

	if _, exists := sessions[session.ID]; !exists {
		t.Error("expected session to be stored in manager")
	}
}

func TestUploadManager_GetUploadSession(t *testing.T) {
	manager := NewUploadManager()
	client := NewClient("http://example.com", "user", "pass")

	// Add a session
	session, err := manager.AddUploadSession("/local/test.txt", "/remote/test.txt", client)
	if err != nil {
		t.Fatalf("unexpected error adding session: %v", err)
	}

	// Retrieve the session
	retrieved, err := manager.GetUploadSession(session.ID)
	if err != nil {
		t.Fatalf("unexpected error getting session: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected session ID '%s', got '%s'", session.ID, retrieved.ID)
	}

	// Test non-existent session
	_, err = manager.GetUploadSession("non-existent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestUploadManager_PauseResumeUpload(t *testing.T) {
	manager := NewUploadManager()
	client := NewClient("http://example.com", "user", "pass")

	// Add a session
	session, err := manager.AddUploadSession("/local/test.txt", "/remote/test.txt", client)
	if err != nil {
		t.Fatalf("unexpected error adding session: %v", err)
	}

	// Start the upload (change status to running)
	session.Status = StatusRunning
	manager.sessions[session.ID] = session

	// Test pause
	err = manager.PauseUpload(session.ID)
	if err != nil {
		t.Fatalf("unexpected error pausing upload: %v", err)
	}

	updatedSession, _ := manager.GetUploadSession(session.ID)
	if updatedSession.Status != StatusPaused {
		t.Errorf("expected status StatusPaused after pause, got %v", updatedSession.Status)
	}

	// Test resume
	err = manager.ResumeUpload(session.ID)
	if err != nil {
		t.Fatalf("unexpected error resuming upload: %v", err)
	}

	updatedSession, _ = manager.GetUploadSession(session.ID)
	if updatedSession.Status != StatusRunning {
		t.Errorf("expected status StatusRunning after resume, got %v", updatedSession.Status)
	}

	// Test pause non-running session
	session.Status = StatusCompleted
	manager.sessions[session.ID] = session

	err = manager.PauseUpload(session.ID)
	if err == nil {
		t.Error("expected error when pausing non-running session")
	}

	// Test resume non-paused session
	err = manager.ResumeUpload(session.ID)
	if err == nil {
		t.Error("expected error when resuming non-paused session")
	}
}

func TestUploadManager_PauseResumeAll(t *testing.T) {
	manager := NewUploadManager()
	client := NewClient("http://example.com", "user", "pass")

	// Add multiple sessions
	sessions := make([]*UploadSession, 3)
	for i := 0; i < 3; i++ {
		session, err := manager.AddUploadSession(
			fmt.Sprintf("/local/test%d.txt", i),
			fmt.Sprintf("/remote/test%d.txt", i),
			client)
		if err != nil {
			t.Fatalf("unexpected error adding session %d: %v", i, err)
		}
		// Set them as running
		session.Status = StatusRunning
		manager.sessions[session.ID] = session
		sessions[i] = session
	}

	// Test pause all
	manager.PauseAllUploads()

	if !manager.IsGloballyPaused() {
		t.Error("expected manager to be globally paused")
	}

	allSessions := manager.GetUploadSessions()
	for _, session := range allSessions {
		if session.Status != StatusPaused {
			t.Errorf("expected session %s to be paused, got %v", session.ID, session.Status)
		}
	}

	// Test resume all
	manager.ResumeAllUploads()

	if manager.IsGloballyPaused() {
		t.Error("expected manager to not be globally paused after resume")
	}

	allSessions = manager.GetUploadSessions()
	for _, session := range allSessions {
		if session.Status != StatusRunning {
			t.Errorf("expected session %s to be running, got %v", session.ID, session.Status)
		}
	}
}

func TestUploadManager_RemoveUploadSession(t *testing.T) {
	manager := NewUploadManager()
	client := NewClient("http://example.com", "user", "pass")

	// Add a session
	session, err := manager.AddUploadSession("/local/test.txt", "/remote/test.txt", client)
	if err != nil {
		t.Fatalf("unexpected error adding session: %v", err)
	}

	// Try to remove running session (should fail)
	session.Status = StatusRunning
	manager.sessions[session.ID] = session

	err = manager.RemoveUploadSession(session.ID)
	if err == nil {
		t.Error("expected error when removing running session")
	}

	// Complete the session and remove it
	session.Status = StatusCompleted
	manager.sessions[session.ID] = session

	err = manager.RemoveUploadSession(session.ID)
	if err != nil {
		t.Fatalf("unexpected error removing completed session: %v", err)
	}

	// Verify it's removed
	sessions := manager.GetUploadSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after removal, got %d", len(sessions))
	}

	// Try to remove non-existent session
	err = manager.RemoveUploadSession("non-existent")
	if err == nil {
		t.Error("expected error when removing non-existent session")
	}
}

func TestUploadController_WithSessionID(t *testing.T) {
	manager := NewUploadManager()
	sessionID := "test-session-123"

	controller := NewUploadController(sessionID, manager)
	if controller == nil {
		t.Fatal("expected NewUploadController to return non-nil controller")
	}

	if controller.sessionID != sessionID {
		t.Errorf("expected sessionID '%s', got '%s'", sessionID, controller.sessionID)
	}

	if controller.manager != manager {
		t.Error("expected controller to reference the manager")
	}

	if controller.State() != StateRunning {
		t.Errorf("expected initial state StateRunning, got %v", controller.State())
	}
}

func TestUploadController_PauseResume(t *testing.T) {
	manager := NewUploadManager()
	controller := NewUploadController("test-session", manager)

	// Test initial state
	if controller.State() != StateRunning {
		t.Errorf("expected initial state StateRunning, got %v", controller.State())
	}

	// Test pause
	controller.Pause()
	if controller.State() != StatePaused {
		t.Errorf("expected state StatePaused after pause, got %v", controller.State())
	}

	// Test resume
	controller.Resume()
	if controller.State() != StateRunning {
		t.Errorf("expected state StateRunning after resume, got %v", controller.State())
	}
}

func TestUploadStatus_String(t *testing.T) {
	statuses := []UploadStatus{
		StatusQueued,
		StatusRunning,
		StatusPaused,
		StatusCompleted,
		StatusFailed,
	}

	expectedStrings := []string{
		"queued",
		"running",
		"paused",
		"completed",
		"failed",
	}

	for i, status := range statuses {
		if string(status) != expectedStrings[i] {
			t.Errorf("expected status string '%s', got '%s'", expectedStrings[i], string(status))
		}
	}
}

func TestProgressInfo_WithSessionID(t *testing.T) {
	info := ProgressInfo{
		Filename:    "test.txt",
		Current:     50,
		Total:       100,
		Percentage:  50.0,
		ChunkIndex:  1,
		TotalChunks: 4,
		SessionID:   "test-session-123",
	}

	if info.SessionID != "test-session-123" {
		t.Errorf("expected SessionID 'test-session-123', got '%s'", info.SessionID)
	}

	if info.Percentage != 50.0 {
		t.Errorf("expected Percentage 50.0, got %f", info.Percentage)
	}
}

func TestEventInfo_WithSessionID(t *testing.T) {
	info := EventInfo{
		Event:     EventUploadStarted,
		Filename:  "test.txt",
		Path:      "/remote/test.txt",
		Message:   "Upload started",
		SessionID: "test-session-123",
	}

	if info.SessionID != "test-session-123" {
		t.Errorf("expected SessionID 'test-session-123', got '%s'", info.SessionID)
	}

	if info.Event != EventUploadStarted {
		t.Errorf("expected Event EventUploadStarted, got %v", info.Event)
	}
}

// Integration test for multi-client upload coordination
func TestMultiClientUploadCoordination(t *testing.T) {
	manager := NewUploadManager()

	// Create multiple clients
	client1 := NewClient("http://example1.com", "user1", "pass1")
	client2 := NewClient("http://example2.com", "user2", "pass2")

	// Track events and progress
	var events []EventInfo
	var progressUpdates []ProgressInfo

	// Create config with callbacks

	client1.config = &Config{
		ChunkSize:  1024,
		MaxRetries: 3,
		EventFunc: func(info EventInfo) {
			events = append(events, info)
		},
		ProgressFunc: func(info ProgressInfo) {
			progressUpdates = append(progressUpdates, info)
		},
	}

	client2.config = &Config{
		ChunkSize:  1024,
		MaxRetries: 3,
		EventFunc: func(info EventInfo) {
			events = append(events, info)
		},
		ProgressFunc: func(info ProgressInfo) {
			progressUpdates = append(progressUpdates, info)
		},
	}

	// Add multiple upload sessions
	session1, err := manager.AddUploadSession("/local/file1.txt", "/remote/file1.txt", client1)
	if err != nil {
		t.Fatalf("error adding session1: %v", err)
	}

	session2, err := manager.AddUploadSession("/local/file2.txt", "/remote/file2.txt", client2)
	if err != nil {
		t.Fatalf("error adding session2: %v", err)
	}

	session3, err := manager.AddUploadSession("/local/file3.txt", "/remote/file3.txt", client1)
	if err != nil {
		t.Fatalf("error adding session3: %v", err)
	}

	// Verify sessions are created with unique IDs
	if session1.ID == session2.ID || session1.ID == session3.ID || session2.ID == session3.ID {
		t.Error("expected all sessions to have unique IDs")
	}

	// Test coordinated pause
	// Set some sessions as running first
	session1.Status = StatusRunning
	session2.Status = StatusRunning
	session3.Status = StatusRunning
	manager.sessions[session1.ID] = session1
	manager.sessions[session2.ID] = session2
	manager.sessions[session3.ID] = session3

	manager.PauseAllUploads()

	// Verify all sessions are paused
	sessions := manager.GetUploadSessions()
	for _, session := range sessions {
		if session.Status != StatusPaused {
			t.Errorf("expected session %s to be paused, got %v", session.ID, session.Status)
		}
	}

	if !manager.IsGloballyPaused() {
		t.Error("expected manager to be globally paused")
	}

	// Test individual resume
	err = manager.ResumeUpload(session2.ID)
	if err != nil {
		t.Fatalf("error resuming session2: %v", err)
	}

	resumedSession, _ := manager.GetUploadSession(session2.ID)
	if resumedSession.Status != StatusRunning {
		t.Errorf("expected session2 to be running after individual resume, got %v", resumedSession.Status)
	}

	// Test coordinated resume
	manager.ResumeAllUploads()

	if manager.IsGloballyPaused() {
		t.Error("expected manager to not be globally paused after resume all")
	}

	sessions = manager.GetUploadSessions()
	for _, session := range sessions {
		if session.Status != StatusRunning {
			t.Errorf("expected session %s to be running after resume all, got %v", session.ID, session.Status)
		}
	}

	// Test that session IDs are properly set in controllers
	for _, session := range sessions {
		if session.Controller.sessionID != session.ID {
			t.Errorf("expected controller sessionID to match session ID, got %s != %s",
				session.Controller.sessionID, session.ID)
		}
	}

	// Verify sessions can be removed when completed
	session1.Status = StatusCompleted
	manager.sessions[session1.ID] = session1

	err = manager.RemoveUploadSession(session1.ID)
	if err != nil {
		t.Fatalf("error removing completed session: %v", err)
	}

	// Verify it's actually removed
	_, err = manager.GetUploadSession(session1.ID)
	if err == nil {
		t.Error("expected error when getting removed session")
	}

	remainingSessions := manager.GetUploadSessions()
	if len(remainingSessions) != 2 {
		t.Errorf("expected 2 remaining sessions, got %d", len(remainingSessions))
	}
}
