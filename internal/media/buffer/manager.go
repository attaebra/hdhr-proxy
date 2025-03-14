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

	// Low buffer threshold (percentage)
	LowBufferThreshold float64
}

// NewManager creates a new buffer manager.
func NewManager(ringBufferSize, readBufferSize, writeBufferSize int) *Manager {
	return &Manager{
		RingBuffer:         ringbuffer.New(ringBufferSize),
		BufferPool:         &bytebufferpool.Pool{},
		ReadBufferSize:     readBufferSize,
		WriteBufferSize:    writeBufferSize,
		LowBufferThreshold: 0.2, // 20% threshold by default
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

// IsBufferLow checks if the buffer is running low (below threshold).
func (m *Manager) IsBufferLow() bool {
	length := m.RingBuffer.Length()
	capacity := m.RingBuffer.Capacity()

	// Calculate fill percentage
	fillPercentage := float64(length) / float64(capacity)

	return fillPercentage < m.LowBufferThreshold
}

// SetLowBufferThreshold sets the threshold for low buffer detection.
func (m *Manager) SetLowBufferThreshold(threshold float64) {
	if threshold > 0 && threshold < 1.0 {
		m.LowBufferThreshold = threshold
	}
}
