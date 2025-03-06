package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the various logging levels.
type LogLevel int

const (
	// LevelError only logs errors.
	LevelError LogLevel = iota
	// LevelWarn logs warnings and errors.
	LevelWarn
	// LevelInfo logs info, warnings, and errors.
	LevelInfo
	// LevelDebug logs everything.
	LevelDebug
)

var (
	// currentLevel is the current logging level.
	currentLevel = LevelInfo
	// mutex to protect level changes.
	mu sync.RWMutex
	// logger instance.
	logger *log.Logger
)

// init initializes the logger.
func init() {
	logger = log.New(os.Stdout, "", log.LstdFlags)
}

// SetLevel sets the current logging level.
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

// GetLevel returns the current logging level.
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

// LevelFromString converts a string log level to LogLevel.
func LevelFromString(level string) LogLevel {
	level = strings.ToLower(level)
	switch level {
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info":
		return LevelInfo
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

// Debug logs a debug message.
func Debug(format string, v ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if currentLevel >= LevelDebug {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s: %s", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Info logs an info message.
func Info(format string, v ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if currentLevel >= LevelInfo {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s: %s", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Warn logs a warning message.
func Warn(format string, v ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if currentLevel >= LevelWarn {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s: %s", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Error logs an error message.
func Error(format string, v ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if currentLevel >= LevelError {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s: %s", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Fatal logs a fatal error message and exits the program.
func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	logger.Fatalf("%s: %s", time.Now().Format("2006/01/02 15:04:05"), msg)
}

// String returns the string representation of a log level.
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
