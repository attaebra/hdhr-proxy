package logger

import (
	"testing"
)

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

func TestZapLoggerCreation(t *testing.T) {
	// Test that we can create loggers at different levels
	testCases := []LogLevel{
		LevelDebug,
		LevelInfo,
		LevelWarn,
		LevelError,
	}

	for _, level := range testCases {
		t.Run(level.String(), func(t *testing.T) {
			logger := NewZapLogger(level)
			if logger == nil {
				t.Errorf("Expected logger to be non-nil for level %s", level.String())
			}
		})
	}
}

func TestStructuredLogging(_ *testing.T) {
	// Test that structured logging functions don't panic
	SetLevel(LevelDebug)

	// Test basic logging functions
	Debug("Test debug message")
	Info("Test info message")
	Warn("Test warn message")
	Error("Test error message")

	// Test logging with arguments
	Debug("Test debug with args: %s", "value")
	Info("Test info with args: %d", 42)
	Warn("Test warn with args: %v", []string{"a", "b"})
	Error("Test error with args: %f", 3.14)
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
		{LogLevel(99), "info"}, // Invalid level defaults to info
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			if tc.level.String() != tc.expected {
				t.Errorf("Expected level.String() to return %s, got %s", tc.expected, tc.level.String())
			}
		})
	}
}

func TestFieldHelpers(t *testing.T) {
	// Test that field helper functions create proper fields
	stringField := String("key", "value")
	if stringField.Key != "key" || stringField.Value != "value" {
		t.Errorf("String field helper failed: got %+v", stringField)
	}

	intField := Int("number", 42)
	if intField.Key != "number" || intField.Value != 42 {
		t.Errorf("Int field helper failed: got %+v", intField)
	}

	int64Field := Int64("big_number", int64(1000))
	if int64Field.Key != "big_number" || int64Field.Value != int64(1000) {
		t.Errorf("Int64 field helper failed: got %+v", int64Field)
	}
}
