package stream

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestStreamHelper(t *testing.T) {
	helper := NewHelper()

	// Test simple copy
	src := strings.NewReader("test data")
	var dst bytes.Buffer

	ctx := context.Background()
	n, err := helper.Copy(ctx, &dst, src)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	if n != 9 {
		t.Errorf("Expected to copy 9 bytes, got %d", n)
	}

	if dst.String() != "test data" {
		t.Errorf("Expected 'test data', got '%s'", dst.String())
	}
}

func TestStreamHelperWithContext(t *testing.T) {
	helper := NewHelper()

	// Test with canceled context
	src := strings.NewReader("test data")
	var dst bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := helper.Copy(ctx, &dst, src)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestStreamHelperWithActivityUpdate(t *testing.T) {
	helper := NewHelper()

	src := strings.NewReader("test data")
	var dst bytes.Buffer

	activityCount := 0
	activityCallback := func() {
		activityCount++
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := helper.CopyWithActivityUpdate(ctx, &dst, src, activityCallback)

	// Should complete successfully or timeout
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("CopyWithActivityUpdate failed: %v", err)
	}

	if dst.String() != "test data" {
		t.Errorf("Expected 'test data', got '%s'", dst.String())
	}
}
