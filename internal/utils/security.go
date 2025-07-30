// Package utils provides utility functions shared across the application.
package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/attaebra/hdhr-proxy/internal/interfaces"
	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// DefaultSecurityValidator implements the SecurityValidator interface.
type DefaultSecurityValidator struct{}

// Ensure DefaultSecurityValidator implements the SecurityValidator interface.
var _ interfaces.SecurityValidator = (*DefaultSecurityValidator)(nil)

// Common errors.
var (
	ErrPathTraversal     = errors.New("path contains directory traversal attempt")
	ErrPathInvalid       = errors.New("path contains invalid characters")
	ErrPathNotFound      = errors.New("path not found")
	ErrPathNotExecutable = errors.New("path is not executable")
)

// ValidateExecutable checks if a path is a valid executable file.
// It performs security checks to prevent command injection and ensure the file exists and is executable.
func (v *DefaultSecurityValidator) ValidateExecutable(path string) error {
	// Use global logger for backward compatibility in utilities
	// In future, this could accept a logger parameter
	logger.Debug("🔒 Validating executable path",
		logger.String("path", path))

	// Check for directory traversal
	if strings.Contains(path, "..") {
		logger.Error("❌ Path contains directory traversal attempt",
			logger.String("path", path))
		return ErrPathTraversal
	}

	// Validate path characters
	validPath := regexp.MustCompile(`^[a-zA-Z0-9_\-./\\]+$`)
	if !validPath.MatchString(path) {
		logger.Error("❌ Path contains invalid characters",
			logger.String("path", path))
		return ErrPathInvalid
	}

	// Clean and resolve the path
	cleanPath := filepath.Clean(path)

	// Check if file exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Error("❌ Executable not found",
				logger.String("path", cleanPath))
			return fmt.Errorf("%w: %s", ErrPathNotFound, cleanPath)
		}
		return fmt.Errorf("error checking path: %w", err)
	}

	// Check if it's a file and executable (on Unix systems)
	if info.IsDir() {
		logger.Error("❌ Path is a directory, not an executable",
			logger.String("path", cleanPath))
		return fmt.Errorf("%w: is a directory", ErrPathNotExecutable)
	}

	// On Unix-like systems, we would check execute permissions
	// For Windows, we'll just check if it has an executable extension
	if isWindows() {
		ext := strings.ToLower(filepath.Ext(cleanPath))
		if ext != ".exe" && ext != ".bat" && ext != ".cmd" {
			logger.Warn("⚠️  Path may not be executable on Windows",
				logger.String("path", cleanPath))
			// Not blocking on Windows, just warning
		}
	} else if info.Mode()&0111 == 0 {
		// On Unix-like systems, check execute permission
		logger.Error("❌ Path is not executable",
			logger.String("path", cleanPath))
		return fmt.Errorf("%w: no execute permission", ErrPathNotExecutable)
	}

	logger.Debug("✅ Validated executable",
		logger.String("path", cleanPath))
	return nil
}

// ValidatePath performs basic path validation.
func (v *DefaultSecurityValidator) ValidatePath(path string) error {
	// Check for directory traversal
	if strings.Contains(path, "..") {
		return ErrPathTraversal
	}

	// Validate path characters
	validPath := regexp.MustCompile(`^[a-zA-Z0-9_\-./\\]+$`)
	if !validPath.MatchString(path) {
		return ErrPathInvalid
	}

	return nil
}

// SanitizeInput performs basic input sanitization.
func (v *DefaultSecurityValidator) SanitizeInput(input string) string {
	// Remove any control characters and limit length
	sanitized := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1 // Remove control characters
		}
		return r
	}, input)

	// Limit length to prevent memory exhaustion
	if len(sanitized) > 1024 {
		sanitized = sanitized[:1024]
	}

	return sanitized
}

// ValidateExecutable is a standalone function for backward compatibility.
func ValidateExecutable(path string) error {
	validator := &DefaultSecurityValidator{}
	return validator.ValidateExecutable(path)
}

// NewSecurityValidator creates a new DefaultSecurityValidator instance.
// This follows the factory pattern for consistency with other components.
func NewSecurityValidator() interfaces.SecurityValidator {
	return &DefaultSecurityValidator{}
}

// DefaultSecurityValidator provides security validation for the application.

// isWindows detects if running on Windows.
func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}
