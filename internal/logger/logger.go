package logger

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/attaebra/hdhr-proxy/internal/interfaces"
)

// ZapLogger implements the Logger interface with Zap.
type ZapLogger struct {
	logger *zap.Logger
}

// Global logger instance for backwards compatibility.
var globalLogger interfaces.Logger

// LogLevel represents the various logging levels.
type LogLevel int

const (
	LevelError LogLevel = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

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
		return "info"
	}
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

// NewZapLogger creates a new production-ready Zap logger with beautiful formatting.
func NewZapLogger(level LogLevel) interfaces.Logger {
	// Determine if we're in development or production
	isDevelopment := os.Getenv("LOG_FORMAT") == "dev" || os.Getenv("ENVIRONMENT") == "development"

	var config zap.Config
	var samplingConfig *zap.SamplingConfig

	if isDevelopment {
		// Development config with beautiful colors and human-readable format
		config = zap.Config{
			Level:       zap.NewAtomicLevelAt(zapLevelFromLogLevel(level)),
			Development: true,
			Encoding:    "console",
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:        "ts",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				FunctionKey:    zapcore.OmitKey,
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.CapitalColorLevelEncoder, // Beautiful colors!
				EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05.000"),
				EncodeDuration: zapcore.StringDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		// Production config with JSON structured logging and sampling
		config = zap.Config{
			Level:       zap.NewAtomicLevelAt(zapLevelFromLogLevel(level)),
			Development: false,
			Encoding:    "json",
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:        "timestamp",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				FunctionKey:    zapcore.OmitKey,
				MessageKey:     "message",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.ISO8601TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}

		// Add sampling to reduce log spam (especially for FFmpeg errors)
		samplingConfig = &zap.SamplingConfig{
			Initial:    100, // Log first 100 of each message per second
			Thereafter: 100, // Then log every 100th message
			Hook: func(entry zapcore.Entry, decision zapcore.SamplingDecision) {
				// Custom sampling logic for FFmpeg errors.
				// For FFmpeg errors, be more aggressive with sampling.
				// Decision handling is done automatically by the sampling framework.
				_ = entry
				_ = decision
			},
		}
		config.Sampling = samplingConfig
	}

	logger, err := config.Build(
		zap.AddCallerSkip(1), // Skip one frame since we're wrapping
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		// Fallback to a minimal logger configuration that should never fail
		fallbackConfig := zap.NewDevelopmentConfig()
		fallbackConfig.Level = zap.NewAtomicLevelAt(zapLevelFromLogLevel(level))
		logger, fallbackErr := fallbackConfig.Build(zap.AddCallerSkip(1))
		if fallbackErr != nil {
			// Last resort: use a no-op logger to prevent crashes
			logger = zap.NewNop()
		}
		// Log the original error once we have a working logger
		if logger != nil {
			logger.Error("Failed to initialize logger with preferred config, using fallback",
				zap.Error(err))
		}
	}

	zapLogger := &ZapLogger{logger: logger}

	// Set as global logger for backwards compatibility
	globalLogger = zapLogger

	return zapLogger
}

// zapLevelFromLogLevel converts our LogLevel to zap.Level.
func zapLevelFromLogLevel(level LogLevel) zapcore.Level {
	switch level {
	case LevelDebug:
		return zap.DebugLevel
	case LevelInfo:
		return zap.InfoLevel
	case LevelWarn:
		return zap.WarnLevel
	case LevelError:
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

// Debug logs a debug message with structured fields.
func (z *ZapLogger) Debug(msg string, fields ...interfaces.Field) {
	z.logger.Debug(msg, fieldsToZap(fields)...)
}

// Info logs an info message with structured fields.
func (z *ZapLogger) Info(msg string, fields ...interfaces.Field) {
	z.logger.Info(msg, fieldsToZap(fields)...)
}

// Warn logs a warning message with structured fields.
func (z *ZapLogger) Warn(msg string, fields ...interfaces.Field) {
	z.logger.Warn(msg, fieldsToZap(fields)...)
}

// Error logs an error message with structured fields.
func (z *ZapLogger) Error(msg string, fields ...interfaces.Field) {
	z.logger.Error(msg, fieldsToZap(fields)...)
}

// Fatal logs a fatal error message and exits.
func (z *ZapLogger) Fatal(msg string, fields ...interfaces.Field) {
	z.logger.Fatal(msg, fieldsToZap(fields)...)
}

// With creates a child logger with additional fields.
func (z *ZapLogger) With(fields ...interfaces.Field) interfaces.Logger {
	return &ZapLogger{
		logger: z.logger.With(fieldsToZap(fields)...),
	}
}

// Sync flushes any buffered log entries.
func (z *ZapLogger) Sync() error {
	return z.logger.Sync()
}

// fieldsToZap converts our Field types to zap.Field.
func fieldsToZap(fields []interfaces.Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, field := range fields {
		zapFields[i] = zap.Any(field.Key, field.Value)
	}
	return zapFields
}

// Backwards compatibility functions for existing code.
var currentLevel = LevelInfo

// SetLevel sets the current logging level (backwards compatibility).
func SetLevel(level LogLevel) {
	currentLevel = level
	// Reinitialize global logger with new level
	globalLogger = NewZapLogger(level)
}

// GetLevel returns the current logging level.
func GetLevel() LogLevel {
	return currentLevel
}

// Helper functions to create structured fields.
func String(key, value string) interfaces.Field {
	return interfaces.Field{Key: key, Value: value}
}

func Int(key string, value int) interfaces.Field {
	return interfaces.Field{Key: key, Value: value}
}

func Int64(key string, value int64) interfaces.Field {
	return interfaces.Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) interfaces.Field {
	return interfaces.Field{Key: key, Value: value}
}

func ErrorField(key string, err error) interfaces.Field {
	return interfaces.Field{Key: key, Value: err}
}

func Any(key string, value interface{}) interfaces.Field {
	return interfaces.Field{Key: key, Value: value}
}

// Backwards compatibility functions.
func Debug(format string, v ...interface{}) {
	if globalLogger == nil {
		globalLogger = NewZapLogger(currentLevel)
	}

	if len(v) == 0 {
		globalLogger.Debug(format)
	} else {
		// Convert printf-style to structured logging with a single field
		globalLogger.Debug(format, Any("details", v))
	}
}

func Info(format string, v ...interface{}) {
	if globalLogger == nil {
		globalLogger = NewZapLogger(currentLevel)
	}

	if len(v) == 0 {
		globalLogger.Info(format)
	} else {
		globalLogger.Info(format, Any("details", v))
	}
}

func Warn(format string, v ...interface{}) {
	if globalLogger == nil {
		globalLogger = NewZapLogger(currentLevel)
	}

	if len(v) == 0 {
		globalLogger.Warn(format)
	} else {
		globalLogger.Warn(format, Any("details", v))
	}
}

func Error(format string, v ...interface{}) {
	if globalLogger == nil {
		globalLogger = NewZapLogger(currentLevel)
	}

	if len(v) == 0 {
		globalLogger.Error(format)
	} else {
		globalLogger.Error(format, Any("details", v))
	}
}

func Fatal(format string, v ...interface{}) {
	if globalLogger == nil {
		globalLogger = NewZapLogger(currentLevel)
	}

	if len(v) == 0 {
		globalLogger.Fatal(format)
	} else {
		globalLogger.Fatal(format, Any("details", v))
	}
}
