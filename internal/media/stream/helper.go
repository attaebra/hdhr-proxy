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
}

// NewHelper creates a new stream helper.
func NewHelper(bufferManager *buffer.Manager) *Helper {
	return &Helper{
		BufferManager: bufferManager,
	}
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
	var consecutiveEmptyReads int

	// Pre-buffer phase - wait until buffer has some data before starting playback
	// This helps prevent initial stuttering
	preBufferTimeout := time.NewTimer(2 * time.Second)
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
			// Check if we have enough data to start
			bufferLength := h.BufferManager.RingBuffer.Length()
			bufferCapacity := h.BufferManager.RingBuffer.Capacity()

			// Start playback when buffer is at least 10% full or after timeout
			if bufferLength > bufferCapacity/10 {
				preBufferDone = true
			} else {
				time.Sleep(10 * time.Millisecond)
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
				// Reset counter since we have data
				consecutiveEmptyReads = 0

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

				// Use a minimal sleep time when we have data
				time.Sleep(1 * time.Millisecond)
			} else {
				// No data available, increment counter
				consecutiveEmptyReads++

				// Implement an adaptive backoff strategy
				// The more consecutive empty reads, the longer we sleep
				// But cap it at a maximum to ensure responsiveness
				var sleepTime time.Duration
				switch {
				case consecutiveEmptyReads > 20:
					sleepTime = 20 * time.Millisecond // Max sleep time
				case consecutiveEmptyReads > 10:
					sleepTime = 10 * time.Millisecond
				case consecutiveEmptyReads > 5:
					sleepTime = 5 * time.Millisecond
				default:
					sleepTime = 1 * time.Millisecond // Minimum sleep
				}

				time.Sleep(sleepTime)
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
