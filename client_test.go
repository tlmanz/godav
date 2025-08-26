package godav

import (
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
