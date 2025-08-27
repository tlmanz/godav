// Package godav - Memory-efficient buffer management
//
// This file provides buffer pooling functionality to reduce memory allocations
// during upload operations. The BufferPool reuses byte buffers to minimize
// garbage collection pressure and improve performance for large file uploads.
//
// Features:
//   - Reusable byte buffer pooling
//   - Configurable pool size and buffer size
//   - Automatic buffer size validation
//   - Non-blocking buffer acquisition and return
package godav

// BufferPool manages reusable byte buffers to reduce allocations
type BufferPool struct {
	pool chan []byte
	size int64
}

// NewBufferPool creates a new buffer pool with the specified chunk size and pool size
func NewBufferPool(chunkSize int64, poolSize int) *BufferPool {
	return &BufferPool{
		pool: make(chan []byte, poolSize),
		size: chunkSize,
	}
}

// Get retrieves a buffer from the pool or creates a new one
func (bp *BufferPool) Get() []byte {
	select {
	case buf := <-bp.pool:
		return buf
	default:
		return make([]byte, bp.size)
	}
}

// Put returns a buffer to the pool for reuse
func (bp *BufferPool) Put(buf []byte) {
	if int64(len(buf)) != bp.size {
		return // Don't pool buffers of wrong size
	}

	select {
	case bp.pool <- buf:
	default:
		// Pool is full, let GC handle it
	}
}
