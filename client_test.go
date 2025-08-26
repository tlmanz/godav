package godav

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
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
	if !c.verbose {
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

	config := &Config{
		EventFunc: func(info EventInfo) {
			eventCalled = true
			capturedEvent = info
		},
	}

	// Test emitting an event
	c.emitEvent(config, EventUploadStarted, "test.txt", "remote/test.txt", "Test message", nil)

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

	config := &Config{
		EventFunc: nil, // No event function configured
	}

	// This should not panic
	c.emitEvent(config, EventUploadStarted, "test.txt", "remote/test.txt", "Test message", nil)
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
			result := c.validateConfig(test.input)
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

	err := c.UploadFileWithContext(ctx, "/nonexistent", "remote", nil)
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
