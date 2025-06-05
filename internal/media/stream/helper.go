package stream

import (
	"context"
	"io"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/media/buffer"
)

// Helper provides optimized streaming functionality.
type Helper struct {
	BufferManager *buffer.Manager
	TestMode      bool // When true, optimizes for tests rather than production
}

// NewHelper creates a new stream helper.
func NewHelper(bufferManager *buffer.Manager) *Helper {
	return &Helper{
		BufferManager: bufferManager,
		TestMode:      false,
	}
}

// EnableTestMode enables test mode for more efficient testing.
func (h *Helper) EnableTestMode() {
	h.TestMode = true
}

// BufferedCopy performs copying with a ring buffer for smoother streaming.
func (h *Helper) BufferedCopy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	// Reset the ring buffer to ensure it's clean
	h.BufferManager.RingBuffer.Reset()

	// Create channels for communication
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	// Create a context that can be canceled when done
	bufferCtx, cancelBuffer := context.WithCancel(ctx)
	defer cancelBuffer()

	// Start filling the ring buffer from the source
	go func() {
		defer close(doneCh)

		for {
			select {
			case <-bufferCtx.Done():
				return
			default:
				// Get a buffer from the pool
				readBuf := h.BufferManager.GetReadBuffer()

				// Read from source
				n, err := src.Read(readBuf.B)
				if n > 0 {
					// Write to ring buffer
					_, werr := h.BufferManager.RingBuffer.Write(readBuf.B[:n])
					if werr != nil {
						h.BufferManager.ReleaseBuffer(readBuf)
						errCh <- werr
						return
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(readBuf)

				// Handle EOF or errors
				if err != nil {
					if err != io.EOF {
						errCh <- err
					} else {
						errCh <- nil // EOF is not an error
					}
					return
				}
			}
		}
	}()

	// Read from ring buffer and write to destination
	var totalCopied int64

	// Minimal pre-buffering - just ensure some data is present
	// Skip pre-buffering in test mode or use very minimal buffering
	if !h.TestMode {
		preBufferTimeout := time.NewTimer(20 * time.Millisecond) // Reduced from 100ms to 20ms
		preBufferDone := false

		for !preBufferDone {
			select {
			case <-ctx.Done():
				return totalCopied, ctx.Err()
			case err := <-errCh:
				// Source ended during pre-buffering
				preBufferDone = true
				if err != nil {
					return totalCopied, err
				}
			case <-preBufferTimeout.C:
				// Timeout reached, start playback anyway
				preBufferDone = true
			default:
				// Check if we have minimal data to start
				bufferLength := h.BufferManager.RingBuffer.Length()
				bufferCapacity := h.BufferManager.RingBuffer.Capacity()

				// Use much smaller threshold - just need some data present
				var threshold int
				if bufferCapacity < 1024 { // Small buffer (test environment)
					threshold = 1 // Just need some data
				} else {
					// Minimal buffering: 32KB or 1% of capacity, whichever is smaller
					threshold = min(32*1024, bufferCapacity/100)
				}

				if bufferLength > threshold {
					preBufferDone = true
				}
				// No sleep here - immediately recheck
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return totalCopied, ctx.Err()

		case err := <-errCh:
			// Source is done, but we still need to drain the buffer
			for h.BufferManager.RingBuffer.Length() > 0 {
				// Get a write buffer
				writeBuf := h.BufferManager.GetWriteBuffer()

				// Read from ring buffer
				n, rerr := h.BufferManager.RingBuffer.Read(writeBuf.B)
				if n > 0 {
					// Write to destination
					nw, werr := dst.Write(writeBuf.B[:n])
					totalCopied += int64(nw)

					if werr != nil {
						h.BufferManager.ReleaseBuffer(writeBuf)
						return totalCopied, werr
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(writeBuf)

				if rerr != nil {
					break
				}
			}
			return totalCopied, err

		default:
			// Get data from ring buffer if available
			if h.BufferManager.RingBuffer.Length() > 0 {
				// Get a write buffer
				writeBuf := h.BufferManager.GetWriteBuffer()

				// Read from ring buffer
				n, rerr := h.BufferManager.RingBuffer.Read(writeBuf.B)
				if n > 0 {
					// Write to destination
					nw, werr := dst.Write(writeBuf.B[:n])
					totalCopied += int64(nw)

					if werr != nil {
						h.BufferManager.ReleaseBuffer(writeBuf)
						return totalCopied, werr
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(writeBuf)

				if rerr != nil && rerr != io.EOF {
					return totalCopied, rerr
				}

				// NO SLEEP when data is available - immediately check for more data
			} else {
				// No data available - yield CPU briefly but don't introduce significant delay
				select {
				case <-ctx.Done():
					return totalCopied, ctx.Err()
				default:
					// Brief yield to prevent tight loop from consuming 100% CPU
					// but avoid fixed sleep delays that cause stuttering
					time.Sleep(10 * time.Microsecond) // Minimal 0.01ms yield
				}
			}
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BufferedCopyWithActivityUpdate performs copying with a ring buffer for smoother streaming
// and calls the provided activity callback whenever data is written to the destination.
func (h *Helper) BufferedCopyWithActivityUpdate(ctx context.Context, dst io.Writer, src io.Reader, activityCallback func()) (int64, error) {
	// Reset the ring buffer to ensure it's clean
	h.BufferManager.RingBuffer.Reset()

	// Create channels for communication
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	// Create a context that can be canceled when done
	bufferCtx, cancelBuffer := context.WithCancel(ctx)
	defer cancelBuffer()

	// Start filling the ring buffer from the source
	go func() {
		defer close(doneCh)

		for {
			select {
			case <-bufferCtx.Done():
				return
			default:
				// Get a buffer from the pool
				readBuf := h.BufferManager.GetReadBuffer()

				// Read from source
				n, err := src.Read(readBuf.B)
				if n > 0 {
					// Write to ring buffer
					_, werr := h.BufferManager.RingBuffer.Write(readBuf.B[:n])
					if werr != nil {
						h.BufferManager.ReleaseBuffer(readBuf)
						errCh <- werr
						return
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(readBuf)

				// Handle EOF or errors
				if err != nil {
					if err != io.EOF {
						errCh <- err
					} else {
						errCh <- nil // EOF is not an error
					}
					return
				}
			}
		}
	}()

	// Read from ring buffer and write to destination
	var totalCopied int64

	// Minimal pre-buffering - just ensure some data is present
	// Skip pre-buffering in test mode or use very minimal buffering
	if !h.TestMode {
		preBufferTimeout := time.NewTimer(20 * time.Millisecond) // Reduced from 100ms to 20ms
		preBufferDone := false

		for !preBufferDone {
			select {
			case <-ctx.Done():
				return totalCopied, ctx.Err()
			case err := <-errCh:
				// Source ended during pre-buffering
				preBufferDone = true
				if err != nil {
					return totalCopied, err
				}
			case <-preBufferTimeout.C:
				// Timeout reached, start playback anyway
				preBufferDone = true
			default:
				// Check if we have minimal data to start
				bufferLength := h.BufferManager.RingBuffer.Length()
				bufferCapacity := h.BufferManager.RingBuffer.Capacity()

				// Use much smaller threshold - just need some data present
				var threshold int
				if bufferCapacity < 1024 { // Small buffer (test environment)
					threshold = 1 // Just need some data
				} else {
					// Minimal buffering: 32KB or 1% of capacity, whichever is smaller
					threshold = min(32*1024, bufferCapacity/100)
				}

				if bufferLength > threshold {
					preBufferDone = true
				}
				// No sleep here - immediately recheck
			}
		}
	}

	// Track when we last called the activity callback to avoid calling it too frequently
	lastActivityUpdate := time.Now()
	activityUpdateInterval := 5 * time.Second // Update activity every 5 seconds

	for {
		select {
		case <-ctx.Done():
			return totalCopied, ctx.Err()

		case err := <-errCh:
			// Source is done, but we still need to drain the buffer
			for h.BufferManager.RingBuffer.Length() > 0 {
				// Get a write buffer
				writeBuf := h.BufferManager.GetWriteBuffer()

				// Read from ring buffer
				n, rerr := h.BufferManager.RingBuffer.Read(writeBuf.B)
				if n > 0 {
					// Write to destination
					nw, werr := dst.Write(writeBuf.B[:n])
					totalCopied += int64(nw)

					// Call activity callback if enough time has passed
					now := time.Now()
					if now.Sub(lastActivityUpdate) >= activityUpdateInterval {
						activityCallback()
						lastActivityUpdate = now
					}

					if werr != nil {
						h.BufferManager.ReleaseBuffer(writeBuf)
						return totalCopied, werr
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(writeBuf)

				if rerr != nil {
					break
				}
			}
			return totalCopied, err

		default:
			// Get data from ring buffer if available
			if h.BufferManager.RingBuffer.Length() > 0 {
				// Get a write buffer
				writeBuf := h.BufferManager.GetWriteBuffer()

				// Read from ring buffer
				n, rerr := h.BufferManager.RingBuffer.Read(writeBuf.B)
				if n > 0 {
					// Write to destination
					nw, werr := dst.Write(writeBuf.B[:n])
					totalCopied += int64(nw)

					// Call activity callback if enough time has passed
					now := time.Now()
					if now.Sub(lastActivityUpdate) >= activityUpdateInterval {
						activityCallback()
						lastActivityUpdate = now
					}

					if werr != nil {
						h.BufferManager.ReleaseBuffer(writeBuf)
						return totalCopied, werr
					}
				}

				// Release the buffer back to the pool
				h.BufferManager.ReleaseBuffer(writeBuf)

				if rerr != nil && rerr != io.EOF {
					return totalCopied, rerr
				}

				// NO SLEEP when data is available - immediately check for more data
			} else {
				// No data available - yield CPU briefly but don't introduce significant delay
				select {
				case <-ctx.Done():
					return totalCopied, ctx.Err()
				default:
					// Brief yield to prevent tight loop from consuming 100% CPU
					// but avoid fixed sleep delays that cause stuttering
					time.Sleep(10 * time.Microsecond) // Minimal 0.01ms yield
				}
			}
		}
	}
}

// GetBufferStatus returns the current fill status of the ring buffer.
func (h *Helper) GetBufferStatus() (used, capacity int) {
	return h.BufferManager.RingBuffer.Length(), h.BufferManager.RingBuffer.Capacity()
}

// GetBufferFillPercentage returns the buffer fill level as a percentage.
func (h *Helper) GetBufferFillPercentage() float64 {
	used, capacity := h.GetBufferStatus()
	if capacity == 0 {
		return 0
	}
	return float64(used) / float64(capacity) * 100
}
