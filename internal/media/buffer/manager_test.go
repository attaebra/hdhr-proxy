package buffer

import (
	"bytes"
	"testing"
)

func TestBufferManager(t *testing.T) {
	// Create a new buffer manager with test sizes
	ringBufferSize := 1024
	readSize := 64
	writeSize := 128

	manager := NewManager(ringBufferSize, readSize, writeSize)

	// Test that the buffer manager was initialized correctly
	if manager.RingBuffer.Capacity() != ringBufferSize {
		t.Errorf("Expected ring buffer capacity to be %d, got %d",
			ringBufferSize, manager.RingBuffer.Capacity())
	}

	if manager.ReadBufferSize != readSize {
		t.Errorf("Expected read buffer size to be %d, got %d",
			readSize, manager.ReadBufferSize)
	}

	if manager.WriteBufferSize != writeSize {
		t.Errorf("Expected write buffer size to be %d, got %d",
			writeSize, manager.WriteBufferSize)
	}

	// Test getting buffers with the correct sizes
	readBuffer := manager.GetReadBuffer()
	if len(readBuffer.B) != readSize {
		t.Errorf("Expected read buffer to have length %d, got %d",
			readSize, len(readBuffer.B))
	}

	writeBuffer := manager.GetWriteBuffer()
	if len(writeBuffer.B) != writeSize {
		t.Errorf("Expected write buffer to have length %d, got %d",
			writeSize, len(writeBuffer.B))
	}

	// Test ring buffer functionality
	testData := []byte("test data for ring buffer")

	// Write to the ring buffer
	n, err := manager.RingBuffer.Write(testData)
	if err != nil {
		t.Errorf("Failed to write to ring buffer: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, but wrote %d",
			len(testData), n)
	}

	// Check ring buffer length
	if manager.RingBuffer.Length() != len(testData) {
		t.Errorf("Expected ring buffer length to be %d, got %d",
			len(testData), manager.RingBuffer.Length())
	}

	// Read from the ring buffer
	readData := make([]byte, len(testData))
	n, err = manager.RingBuffer.Read(readData)
	if err != nil {
		t.Errorf("Failed to read from ring buffer: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, but read %d",
			len(testData), n)
	}

	// Verify the data matches
	if !bytes.Equal(readData, testData) {
		t.Errorf("Data mismatch. Expected: %s, Got: %s",
			testData, readData)
	}

	// Test releasing buffers back to the pool
	manager.ReleaseBuffer(readBuffer)
	manager.ReleaseBuffer(writeBuffer)

	// The ByteBuffer from bytebufferpool doesn't necessarily have a length of 0 after ReleaseBuffer
	// It's just returned to the pool for reuse, so we won't check its length here

	// Clean up by resetting the ring buffer
	manager.RingBuffer.Reset()
}
