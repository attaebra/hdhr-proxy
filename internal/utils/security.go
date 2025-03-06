// Package utils provides utility functions shared across the application.
package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// Common errors.
var (
	ErrPathTraversal     = errors.New("path contains directory traversal attempt")
	ErrPathInvalid       = errors.New("path contains invalid characters")
	ErrPathNotFound      = errors.New("path not found")
	ErrPathNotExecutable = errors.New("path is not executable")
)

// ValidateExecutable checks if a path is a valid executable file.
// It performs security checks to prevent command injection and ensure the file exists and is executable.
func ValidateExecutable(path string) error {
	logger.Debug("Validating executable path: %s", path)

	// Check for directory traversal
	if strings.Contains(path, "..") {
		logger.Error("Path contains directory traversal attempt: %s", path)
		return ErrPathTraversal
	}

	// Validate path characters
	validPath := regexp.MustCompile(`^[a-zA-Z0-9_\-./\\]+$`)
	if !validPath.MatchString(path) {
		logger.Error("Path contains invalid characters: %s", path)
		return ErrPathInvalid
	}

	// Clean and resolve the path
	cleanPath := filepath.Clean(path)

	// Check if file exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Error("Executable not found: %s", cleanPath)
			return fmt.Errorf("%w: %s", ErrPathNotFound, cleanPath)
		}
		return fmt.Errorf("error checking path: %w", err)
	}

	// Check if it's a file and executable (on Unix systems)
	if info.IsDir() {
		logger.Error("Path is a directory, not an executable: %s", cleanPath)
		return fmt.Errorf("%w: is a directory", ErrPathNotExecutable)
	}

	// On Unix-like systems, we would check execute permissions
	// For Windows, we'll just check if it has an executable extension
	if isWindows() {
		ext := strings.ToLower(filepath.Ext(cleanPath))
		if ext != ".exe" && ext != ".bat" && ext != ".cmd" {
			logger.Warn("Path may not be executable on Windows: %s", cleanPath)
			// Not blocking on Windows, just warning
		}
	} else if info.Mode()&0111 == 0 {
		// On Unix-like systems, check execute permission
		logger.Error("Path is not executable: %s", cleanPath)
		return fmt.Errorf("%w: no execute permission", ErrPathNotExecutable)
	}

	logger.Debug("Validated executable: %s", cleanPath)
	return nil
}

// isWindows detects if running on Windows.
func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}
