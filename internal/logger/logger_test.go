package logger

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// Capture log output by replacing the logger.
func captureOutput(f func()) string {
	// Create a buffer to store the log output.
	var buf bytes.Buffer

	// Save the original logger.
	origLogger := logger

	// Replace with a logger writing to our buffer.
	logger = log.New(&buf, "", log.LstdFlags)

	// Call the function that produces log output.
	f()

	// Restore the original logger.
	logger = origLogger

	// Return the captured output.
	return buf.String()
}

func TestLevelFromString(t *testing.T) {
	// Test with various log levels
	testCases := []struct {
		level    string
		expected LogLevel
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"invalid", LevelInfo}, // Default to INFO for invalid levels
	}

	for _, tc := range testCases {
		t.Run(tc.level, func(t *testing.T) {
			level := LevelFromString(tc.level)

			if level != tc.expected {
				t.Errorf("Expected log level to be %v for '%s', got %v", tc.expected, tc.level, level)
			}
		})
	}
}

func TestSetAndGetLevel(t *testing.T) {
	// Test setting and getting levels
	testCases := []LogLevel{
		LevelDebug,
		LevelInfo,
		LevelWarn,
		LevelError,
	}

	for _, tc := range testCases {
		t.Run(tc.String(), func(t *testing.T) {
			SetLevel(tc)

			if GetLevel() != tc {
				t.Errorf("Expected GetLevel() to return %v, got %v", tc, GetLevel())
			}
		})
	}
}

func TestDebug(t *testing.T) {
	// Test with debug level enabled.
	SetLevel(LevelDebug)
	output := captureOutput(func() {
		Debug("Test debug message: %s", "hello")
	})

	if !strings.Contains(output, "Test debug message: hello") {
		t.Errorf("Debug log doesn't contain expected content: %s", output)
	}

	// Test with debug level disabled.
	SetLevel(LevelInfo)
	output = captureOutput(func() {
		Debug("This should not appear")
	})

	if output != "" {
		t.Errorf("Expected empty output when debug is disabled, got: %s", output)
	}
}

func TestInfo(t *testing.T) {
	SetLevel(LevelInfo)
	output := captureOutput(func() {
		Info("Test info message: %s", "hello")
	})

	if !strings.Contains(output, "Test info message: hello") {
		t.Errorf("Info log doesn't contain expected content: %s", output)
	}
}

func TestWarn(t *testing.T) {
	// Test with warn level enabled.
	SetLevel(LevelWarn)
	output := captureOutput(func() {
		Warn("Test warn message: %s", "hello")
	})

	if !strings.Contains(output, "Test warn message: hello") {
		t.Errorf("Warn log doesn't contain expected content: %s", output)
	}

	// Test with warn level disabled.
	SetLevel(LevelError)
	output = captureOutput(func() {
		Warn("This should not appear")
	})

	if output != "" {
		t.Errorf("Expected empty output when warn is disabled, got: %s", output)
	}
}

func TestError(t *testing.T) {
	// Test with error level enabled.
	SetLevel(LevelError)
	output := captureOutput(func() {
		Error("Test error message: %s", "hello")
	})

	if !strings.Contains(output, "Test error message: hello") {
		t.Errorf("Error log doesn't contain expected content: %s", output)
	}
}

func TestLogLevelString(t *testing.T) {
	testCases := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LogLevel(99), "LogLevel(99)"}, // Invalid level
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			if tc.level.String() != tc.expected {
				t.Errorf("Expected level.String() to return %s, got %s", tc.expected, tc.level.String())
			}
		})
	}
}
