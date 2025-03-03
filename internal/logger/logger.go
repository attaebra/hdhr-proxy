package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// LogLevel represents the various logging levels
type LogLevel int

const (
	// LevelError only logs errors
	LevelError LogLevel = iota
	// LevelWarn logs warnings and errors
	LevelWarn
	// LevelInfo logs info, warnings, and errors
	LevelInfo
	// LevelDebug logs everything
	LevelDebug
)

var (
	// currentLevel is the current logging level
	currentLevel LogLevel = LevelInfo
	// mutex to protect level changes
	mu sync.RWMutex
	// logger instance
	logger *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "", log.LstdFlags)
}

// SetLevel sets the current logging level
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

// GetLevel returns the current logging level
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

// LevelFromString converts a string log level to LogLevel
func LevelFromString(level string) LogLevel {
	switch level {
	case "error":
		return LevelError
	case "warn":
		return LevelWarn
	case "info":
		return LevelInfo
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

// Debug logs a debug message
func Debug(format string, v ...interface{}) {
	mu.RLock()
	shouldLog := currentLevel >= LevelDebug
	mu.RUnlock()

	if shouldLog {
		logger.Printf("DEBUG: "+format, v...)
	}
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	mu.RLock()
	shouldLog := currentLevel >= LevelInfo
	mu.RUnlock()

	if shouldLog {
		logger.Printf("INFO: "+format, v...)
	}
}

// Warn logs a warning message
func Warn(format string, v ...interface{}) {
	mu.RLock()
	shouldLog := currentLevel >= LevelWarn
	mu.RUnlock()

	if shouldLog {
		logger.Printf("WARN: "+format, v...)
	}
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	mu.RLock()
	shouldLog := currentLevel >= LevelError
	mu.RUnlock()

	if shouldLog {
		logger.Printf("ERROR: "+format, v...)
	}
}

// Fatal logs a fatal error message and exits
func Fatal(format string, v ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()

	logger.Fatalf("FATAL: "+format, v...)
}

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	default:
		return fmt.Sprintf("LogLevel(%d)", l)
	}
}
