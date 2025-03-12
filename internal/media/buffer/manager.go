package buffer

import (
	"github.com/smallnest/ringbuffer"
	"github.com/valyala/bytebufferpool"
)

// Manager handles buffering for media streaming.
type Manager struct {
	// Ring buffer for smooth streaming
	RingBuffer *ringbuffer.RingBuffer

	// Buffer pool for memory efficiency
	BufferPool *bytebufferpool.Pool

	// Buffer sizes
	ReadBufferSize  int
	WriteBufferSize int
}

// NewManager creates a new buffer manager.
func NewManager(ringBufferSize, readBufferSize, writeBufferSize int) *Manager {
	return &Manager{
		RingBuffer:      ringbuffer.New(ringBufferSize),
		BufferPool:      &bytebufferpool.Pool{},
		ReadBufferSize:  readBufferSize,
		WriteBufferSize: writeBufferSize,
	}
}

// GetReadBuffer gets a buffer for reading from the pool.
func (m *Manager) GetReadBuffer() *bytebufferpool.ByteBuffer {
	buf := m.BufferPool.Get()
	// Ensure it has enough capacity
	if cap(buf.B) < m.ReadBufferSize {
		buf.B = make([]byte, m.ReadBufferSize)
	} else {
		buf.B = buf.B[:m.ReadBufferSize]
	}
	return buf
}

// GetWriteBuffer gets a buffer for writing from the pool.
func (m *Manager) GetWriteBuffer() *bytebufferpool.ByteBuffer {
	buf := m.BufferPool.Get()
	// Ensure it has enough capacity
	if cap(buf.B) < m.WriteBufferSize {
		buf.B = make([]byte, m.WriteBufferSize)
	} else {
		buf.B = buf.B[:m.WriteBufferSize]
	}
	return buf
}

// ReleaseBuffer returns a buffer to the pool.
func (m *Manager) ReleaseBuffer(buf *bytebufferpool.ByteBuffer) {
	buf.Reset()
	m.BufferPool.Put(buf)
}
