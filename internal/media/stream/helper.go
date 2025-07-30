package stream

import (
	"context"
	"io"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/interfaces"
)

// Helper provides simple streaming functionality.
type Helper struct{}

// Ensure Helper implements the StreamHelper interface.
var _ interfaces.StreamHelper = (*Helper)(nil)

// NewHelper creates a new stream helper.
func NewHelper() *Helper {
	return &Helper{}
}

// Copy performs simple copying with context cancellation support.
func (h *Helper) Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	// Use a goroutine to handle the copy and make it cancellable
	type result struct {
		n   int64
		err error
	}

	resultCh := make(chan result, 1)

	go func() {
		n, err := io.Copy(dst, src)
		resultCh <- result{n, err}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case res := <-resultCh:
		return res.n, res.err
	}
}

// CopyWithActivityUpdate performs copying with activity callback.
func (h *Helper) CopyWithActivityUpdate(ctx context.Context, dst io.Writer, src io.Reader, activityCallback func()) (int64, error) {
	// Use a goroutine to handle the copy and make it cancellable
	type result struct {
		n   int64
		err error
	}

	resultCh := make(chan result, 1)

	go func() {
		n, err := io.Copy(dst, src)
		resultCh <- result{n, err}
	}()

	// Start activity updater
	activityTicker := time.NewTicker(1 * time.Second)
	defer activityTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-activityTicker.C:
			activityCallback()
		case res := <-resultCh:
			return res.n, res.err
		}
	}
}
