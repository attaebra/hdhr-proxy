package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/media/buffer"
)

// mockReader implements io.Reader for testing
type mockReader struct {
	data        []byte
	chunkSize   int
	currentPos  int
	readDelay   time.Duration // Delay before each read to simulate network latency
	errorAfter  int           // Return error after this many bytes, -1 for no error
	errorToSend error         // Error to return when errorAfter is reached
}

func newMockReader(data []byte, chunkSize int) *mockReader {
	return &mockReader{
		data:        data,
		chunkSize:   chunkSize,
		currentPos:  0,
		readDelay:   0,
		errorAfter:  -1,
		errorToSend: nil,
	}
}

func (m *mockReader) withDelay(delay time.Duration) *mockReader {
	m.readDelay = delay
	return m
}

func (m *mockReader) withErrorAfter(pos int, err error) *mockReader {
	m.errorAfter = pos
	m.errorToSend = err
	return m
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	// Apply read delay to simulate network conditions
	if m.readDelay > 0 {
		time.Sleep(m.readDelay)
	}

	// Check if we should return an error
	if m.errorAfter >= 0 && m.currentPos >= m.errorAfter {
		return 0, m.errorToSend
	}

	// Check if we're at the end
	if m.currentPos >= len(m.data) {
		return 0, io.EOF
	}

	// Calculate how much data to read
	remainingData := len(m.data) - m.currentPos
	readSize := m.chunkSize
	if readSize > len(p) {
		readSize = len(p)
	}
	if readSize > remainingData {
		readSize = remainingData
	}

	// Copy data to the buffer
	copy(p, m.data[m.currentPos:m.currentPos+readSize])
	m.currentPos += readSize
	return readSize, nil
}

// mockWriter implements io.Writer for testing
type mockWriter struct {
	buffer       bytes.Buffer
	writeDelay   time.Duration // Delay before each write to simulate network latency
	errorAfter   int           // Return error after this many bytes, -1 for no error
	errorToSend  error         // Error to return when errorAfter is reached
	bytesWritten int
}

func newMockWriter() *mockWriter {
	return &mockWriter{
		writeDelay:   0,
		errorAfter:   -1,
		errorToSend:  nil,
		bytesWritten: 0,
	}
}

func (m *mockWriter) withDelay(delay time.Duration) *mockWriter {
	m.writeDelay = delay
	return m
}

func (m *mockWriter) withErrorAfter(pos int, err error) *mockWriter {
	m.errorAfter = pos
	m.errorToSend = err
	return m
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	// Apply write delay to simulate network conditions
	if m.writeDelay > 0 {
		time.Sleep(m.writeDelay)
	}

	// Check if we should return an error
	if m.errorAfter >= 0 && m.bytesWritten+len(p) > m.errorAfter {
		errorPos := m.errorAfter - m.bytesWritten
		if errorPos > 0 {
			// Write partial data before error
			n, _ = m.buffer.Write(p[:errorPos])
			m.bytesWritten += n
		}
		return n, m.errorToSend
	}

	// Write data to buffer
	n, err = m.buffer.Write(p)
	m.bytesWritten += n
	return n, err
}

func (m *mockWriter) String() string {
	return m.buffer.String()
}

func (m *mockWriter) Bytes() []byte {
	return m.buffer.Bytes()
}

// TestBufferedCopyBasic tests basic functionality of the BufferedCopy method
func TestBufferedCopyBasic(t *testing.T) {
	// Create test data
	testData := []byte("This is test data for buffered copying. It should be copied correctly from source to destination.")
	
	// Create a mock reader and writer
	reader := newMockReader(testData, 16) // Read in 16-byte chunks
	writer := newMockWriter()
	
	// Create a buffer manager with small sizes for testing
	bufferManager := buffer.NewManager(128, 32, 32)
	
	// Create the stream helper
	helper := NewHelper(bufferManager)
	
	// Create a context
	ctx := context.Background()
	
	// Perform the buffered copy
	copied, err := helper.BufferedCopy(ctx, writer, reader)
	
	// Verify results
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if copied != int64(len(testData)) {
		t.Errorf("Expected to copy %d bytes, but copied %d", len(testData), copied)
	}
	
	if !bytes.Equal(writer.Bytes(), testData) {
		t.Errorf("Copied data doesn't match original data")
	}
}

// TestBufferedCopyWithContextCancellation tests that BufferedCopy respects context cancellation
func TestBufferedCopyWithContextCancellation(t *testing.T) {
	// Create a larger test data set
	testData := make([]byte, 100*1024) // 100KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Create a mock reader with delay to ensure we can cancel before completion
	reader := newMockReader(testData, 1024).withDelay(5 * time.Millisecond)
	writer := newMockWriter()
	
	// Create a buffer manager with small sizes for testing
	bufferManager := buffer.NewManager(8*1024, 1024, 1024)
	
	// Create the stream helper
	helper := NewHelper(bufferManager)
	
	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create a channel to signal when copy is done
	done := make(chan struct{})
	
	// Start the copy operation in a goroutine
	var copied int64
	var err error
	go func() {
		copied, err = helper.BufferedCopy(ctx, writer, reader)
		close(done)
	}()
	
	// Cancel the context after a short time
	time.Sleep(20 * time.Millisecond)
	cancel()
	
	// Wait for the operation to complete
	select {
	case <-done:
		// Operation completed
	case <-time.After(1 * time.Second):
		t.Fatal("BufferedCopy did not respect context cancellation")
	}
	
	// Verify results
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
	
	if copied >= int64(len(testData)) {
		t.Errorf("Expected partial copy, but copied all %d bytes", copied)
	}
}

// TestBufferedCopyWithReadError tests handling of read errors
func TestBufferedCopyWithReadError(t *testing.T) {
	// Create smaller test data to avoid ring buffer getting full
	testData := make([]byte, 4*1024) // 4KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Create a custom error
	customError := errors.New("simulated read error")
	
	// Create a mock reader that returns an error after 2KB
	reader := newMockReader(testData, 512).withErrorAfter(2*1024, customError)
	writer := newMockWriter()
	
	// Create a buffer manager with smaller sizes for testing
	bufferManager := buffer.NewManager(4*1024, 512, 512)
	
	// Create the stream helper
	helper := NewHelper(bufferManager)
	
	// Create a context
	ctx := context.Background()
	
	// Perform the buffered copy
	copied, err := helper.BufferedCopy(ctx, writer, reader)
	
	// Verify results
	if err != customError {
		t.Errorf("Expected custom error, got: %v", err)
	}
	
	if copied != 2*1024 {
		t.Errorf("Expected to copy 2KB before error, but copied %d bytes", copied)
	}
}

// TestBufferedCopyWithWriteError tests handling of write errors
func TestBufferedCopyWithWriteError(t *testing.T) {
	// Create smaller test data to avoid ring buffer getting full
	testData := make([]byte, 4*1024) // 4KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Create a custom error
	customError := errors.New("simulated write error")
	
	// Create a mock reader and writer that returns an error after 2KB
	reader := newMockReader(testData, 512) 
	writer := newMockWriter().withErrorAfter(2*1024, customError)
	
	// Create a buffer manager with smaller sizes for testing
	bufferManager := buffer.NewManager(4*1024, 512, 512)
	
	// Create the stream helper
	helper := NewHelper(bufferManager)
	
	// Create a context
	ctx := context.Background()
	
	// Perform the buffered copy
	copied, err := helper.BufferedCopy(ctx, writer, reader)
	
	// Verify results
	if err != customError {
		t.Errorf("Expected custom error, got: %v", err)
	}
	
	if copied > 2*1024 {
		t.Errorf("Expected to copy no more than 2KB before error, but copied %d bytes", copied)
	}
}

// TestGetBufferStatus tests the GetBufferStatus method
func TestGetBufferStatus(t *testing.T) {
	// Create a buffer manager with known size
	ringBufferSize := 1024
	bufferManager := buffer.NewManager(ringBufferSize, 64, 64)
	
	// Create the stream helper
	helper := NewHelper(bufferManager)
	
	// Check initial status
	used, capacity := helper.GetBufferStatus()
	if used != 0 {
		t.Errorf("Expected 0 bytes used initially, got %d", used)
	}
	
	if capacity != ringBufferSize {
		t.Errorf("Expected %d bytes capacity, got %d", ringBufferSize, capacity)
	}
	
	// Write some data to the ring buffer
	testData := []byte("test data for buffer status")
	_, err := bufferManager.RingBuffer.Write(testData)
	if err != nil {
		t.Errorf("Failed to write to ring buffer: %v", err)
	}
	
	// Check updated status
	used, capacity = helper.GetBufferStatus()
	if used != len(testData) {
		t.Errorf("Expected %d bytes used, got %d", len(testData), used)
	}
	
	if capacity != ringBufferSize {
		t.Errorf("Expected %d bytes capacity, got %d", ringBufferSize, capacity)
	}
} 