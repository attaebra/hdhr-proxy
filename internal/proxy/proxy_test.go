package proxy

import (
	"testing"
)

func TestReverseDeviceID(t *testing.T) {
	testCases := []struct {
		name     string
		deviceID string
		expected string
	}{
		{
			name:     "Empty string",
			deviceID: "",
			expected: "",
		},
		{
			name:     "Single character",
			deviceID: "A",
			expected: "A",
		},
		{
			name:     "Standard device ID",
			deviceID: "00ABCDEF",
			expected: "FEDCBA00",
		},
		{
			name:     "Palindrome",
			deviceID: "ABCCBA",
			expected: "ABCCBA",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := &HDHRProxy{DeviceID: tc.deviceID}
			result := proxy.ReverseDeviceID()
			if result != tc.expected {
				t.Errorf("ReverseDeviceID() = %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestTransformResponseBody(t *testing.T) {
	proxy := &HDHRProxy{
		HDHRIP:   "192.168.1.100",
		DeviceID: "00ABCDEF",
	}

	testCases := []struct {
		name     string
		input    string
		host     string
		expected string
	}{
		{
			name:     "Replace device ID",
			input:    "Device 00ABCDEF is ready",
			host:     "localhost:80",
			expected: "Device FEDCBA00 is ready",
		},
		{
			name:     "Replace HDHR IP without port",
			input:    "Connect to 192.168.1.100 for service",
			host:     "localhost:80",
			expected: "Connect to localhost for service",
		},
		{
			name:     "Replace HDHR IP with port 5004",
			input:    "Media at 192.168.1.100:5004/auto/v5.1",
			host:     "localhost:80",
			expected: "Media at localhost:5004/auto/v5.1",
		},
		{
			name:     "Replace AC4 with AC3",
			input:    "Audio format: AC4",
			host:     "localhost:80",
			expected: "Audio format: AC3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := string(proxy.transformResponseBody([]byte(tc.input), tc.host))
			if result != tc.expected {
				t.Errorf("transformResponseBody() =\n%v, want\n%v", result, tc.expected)
			}
		})
	}
} 