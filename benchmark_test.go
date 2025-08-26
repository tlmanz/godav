package godav

import (
	"encoding/json"
	"testing"
	"time"
)

func BenchmarkBufferPool(b *testing.B) {
	chunkSize := int64(1024 * 1024) // 1MB
	pool := NewBufferPool(chunkSize, 10)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			// Simulate some work
			for i := 0; i < len(buf); i += 1024 {
				buf[i] = byte(i)
			}
			pool.Put(buf)
		}
	})
}

func BenchmarkDirectAllocation(b *testing.B) {
	chunkSize := int64(1024 * 1024) // 1MB

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := make([]byte, chunkSize)
			// Simulate some work
			for i := 0; i < len(buf); i += 1024 {
				buf[i] = byte(i)
			}
			// buf goes out of scope and gets GC'd
		}
	})
}

func BenchmarkCalculateChunks(b *testing.B) {
	fileSize := int64(100 * 1024 * 1024) // 100MB
	chunkSize := int64(10 * 1024 * 1024) // 10MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculateChunks(fileSize, chunkSize)
	}
}

func BenchmarkPathJoin(b *testing.B) {
	c := NewClient("http://example.com", "user", "pass")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.pathJoin("path/to/dir", "file.txt")
	}
}

func BenchmarkValidateConfig(b *testing.B) {
	c := NewClient("http://example.com", "user", "pass")
	config := &Config{
		ChunkSize:  1024 * 1024,
		MaxRetries: 3,
		Verbose:    true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.validateConfig(config)
	}
}

func BenchmarkUploadController(b *testing.B) {
	controller := NewUploadController()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		controller.Pause()
		controller.Resume()
		_ = controller.State()
	}
}

func BenchmarkCheckpointSerialization(b *testing.B) {
	checkpoint := Checkpoint{
		LocalPath:      "/test/file.txt",
		RemotePath:     "remote/file.txt", 
		UploadID:       "test-123",
		FileSize:       1000000,
		ChunkSize:      10240,
		BytesUploaded:  500000,
		ChunksUploaded: 48,
		TotalChunks:    97,
		Timestamp:      time.Now(),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(checkpoint)
		var cp Checkpoint
		_ = json.Unmarshal(data, &cp)
	}
}
