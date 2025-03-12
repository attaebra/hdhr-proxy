package ffmpeg

import (
	"testing"
)

func TestNewOptimizedConfig(t *testing.T) {
	// Create a new config with optimized settings
	config := NewOptimizedConfig()

	// Verify all the expected fields are set correctly
	expectedFields := map[string]string{
		"InputSource":        "pipe:0",
		"OutputTarget":       "pipe:1",
		"VideoCodec":         "copy",
		"AudioCodec":         "eac3",
		"AudioBitrate":       "384k",
		"AudioChannels":      "2",
		"BufferSize":         "12288k",
		"MaxRate":            "30M",
		"Preset":             "superfast",
		"Tune":               "zerolatency",
		"ThreadQueueSize":    "4096",
		"MaxMuxingQueueSize": "1024",
		"Threads":            "4",
		"Format":             "mpegts",
	}

	// Check each field
	if config.InputSource != expectedFields["InputSource"] {
		t.Errorf("Expected InputSource to be %s, got %s", expectedFields["InputSource"], config.InputSource)
	}

	if config.OutputTarget != expectedFields["OutputTarget"] {
		t.Errorf("Expected OutputTarget to be %s, got %s", expectedFields["OutputTarget"], config.OutputTarget)
	}

	if config.VideoCodec != expectedFields["VideoCodec"] {
		t.Errorf("Expected VideoCodec to be %s, got %s", expectedFields["VideoCodec"], config.VideoCodec)
	}

	if config.AudioCodec != expectedFields["AudioCodec"] {
		t.Errorf("Expected AudioCodec to be %s, got %s", expectedFields["AudioCodec"], config.AudioCodec)
	}

	if config.AudioBitrate != expectedFields["AudioBitrate"] {
		t.Errorf("Expected AudioBitrate to be %s, got %s", expectedFields["AudioBitrate"], config.AudioBitrate)
	}

	if config.AudioChannels != expectedFields["AudioChannels"] {
		t.Errorf("Expected AudioChannels to be %s, got %s", expectedFields["AudioChannels"], config.AudioChannels)
	}

	if config.BufferSize != expectedFields["BufferSize"] {
		t.Errorf("Expected BufferSize to be %s, got %s", expectedFields["BufferSize"], config.BufferSize)
	}

	if config.MaxRate != expectedFields["MaxRate"] {
		t.Errorf("Expected MaxRate to be %s, got %s", expectedFields["MaxRate"], config.MaxRate)
	}

	if config.Preset != expectedFields["Preset"] {
		t.Errorf("Expected Preset to be %s, got %s", expectedFields["Preset"], config.Preset)
	}

	if config.Tune != expectedFields["Tune"] {
		t.Errorf("Expected Tune to be %s, got %s", expectedFields["Tune"], config.Tune)
	}

	if config.ThreadQueueSize != expectedFields["ThreadQueueSize"] {
		t.Errorf("Expected ThreadQueueSize to be %s, got %s", expectedFields["ThreadQueueSize"], config.ThreadQueueSize)
	}

	if config.MaxMuxingQueueSize != expectedFields["MaxMuxingQueueSize"] {
		t.Errorf("Expected MaxMuxingQueueSize to be %s, got %s", expectedFields["MaxMuxingQueueSize"], config.MaxMuxingQueueSize)
	}

	if config.Threads != expectedFields["Threads"] {
		t.Errorf("Expected Threads to be %s, got %s", expectedFields["Threads"], config.Threads)
	}

	if config.Format != expectedFields["Format"] {
		t.Errorf("Expected Format to be %s, got %s", expectedFields["Format"], config.Format)
	}
}

func TestBuildArgs(t *testing.T) {
	// Create a new config
	config := NewOptimizedConfig()

	// Build the arguments
	args := config.BuildArgs()

	// Count the actual parameters
	expectedParameters := []string{
		"-i", "pipe:0",
		"-c:v", "copy",
		"-c:a", "eac3",
		"-b:a", "384k",
		"-ac", "2",
		"-bufsize", "12288k",
		"-maxrate", "30M",
		"-preset", "superfast",
		"-tune", "zerolatency",
		"-thread_queue_size", "4096",
		"-max_muxing_queue_size", "1024",
		"-threads", "4",
		"-f", "mpegts",
		"pipe:1",
	}

	// The count should match our list of expected parameters
	if len(args) != len(expectedParameters) {
		t.Errorf("Expected %d arguments, got %d", len(expectedParameters), len(args))
	}

	// Verify key parameters are present and in the right order
	if args[0] != "-i" || args[1] != "pipe:0" {
		t.Errorf("Expected first parameter to be -i pipe:0, got %s %s", args[0], args[1])
	}

	// Check for specific optimization parameters
	tuneArgIndex := -1
	for i, arg := range args {
		if arg == "-tune" && i+1 < len(args) {
			tuneArgIndex = i
			break
		}
	}

	if tuneArgIndex == -1 {
		t.Errorf("Expected -tune parameter, but it was not found")
	} else if args[tuneArgIndex+1] != "zerolatency" {
		t.Errorf("Expected -tune zerolatency, got -tune %s", args[tuneArgIndex+1])
	}

	// Verify the last parameter is the output target
	if args[len(args)-1] != "pipe:1" {
		t.Errorf("Expected last parameter to be pipe:1, got %s", args[len(args)-1])
	}
}
