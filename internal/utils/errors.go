// Package utils provides utility functions shared across the application.
package utils

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// LogAndWrapError logs an error and returns a formatted error with the original wrapped.
// This function ensures consistent error handling and logging throughout the application.
func LogAndWrapError(err error, format string, args ...interface{}) error {
	if err != nil {
		// Convert to structured logging
		logger.Error("❌ "+format,
			logger.ErrorField("error", err),
			logger.Any("args", args))
		return fmt.Errorf(format+": %w", append(args, err)...)
	}
	return nil
}

// LogAndReturnWithHTTPError logs an error, sends an HTTP error response, and returns a wrapped error.
// This function is used in HTTP handlers where both logging and response writing are needed.
func LogAndReturnWithHTTPError(w http.ResponseWriter, status int, err error, logFormat string, userMessage string, args ...interface{}) error {
	if err != nil {
		// Log the error with structured logging
		logger.Error("❌ HTTP error: "+logFormat,
			logger.ErrorField("error", err),
			logger.Int("status_code", status),
			logger.String("user_message", userMessage),
			logger.Any("args", args))

		// Send HTTP error response with user-friendly message
		http.Error(w, userMessage, status)

		// Return a wrapped error for the caller
		return fmt.Errorf(logFormat+": %w", append(args, err)...)
	}
	return nil
}

// HandleClientDisconnect checks if an error is due to client disconnection and handles it appropriately.
// Returns true if the error was a client disconnection, false otherwise.
func HandleClientDisconnect(err error, channel string) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	if isClientDisconnectError(errStr) {
		logger.Debug("🔌 Client disconnected",
			logger.String("channel", channel),
			logger.ErrorField("error", err))
		return true
	}
	return false
}

// isClientDisconnectError determines if an error string indicates a client disconnection.
func isClientDisconnectError(errStr string) bool {
	return contains(errStr,
		"connection reset by peer",
		"broken pipe",
		"client disconnected",
		"i/o timeout",
		"use of closed network connection")
}

// contains checks if a string contains any of the provided substrings.
func contains(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if substr != "" && strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
