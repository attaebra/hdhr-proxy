package ffmpeg

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	// Test that New returns a valid configuration
	config := New()

	// Verify all the expected fields are set correctly (anti-stuttering values)
	expectedFields := map[string]string{
		"InputSource":        "pipe:0",
		"OutputTarget":       "pipe:1",
		"VideoCodec":         "copy",
		"AudioCodec":         "eac3",
		"AudioBitrate":       "384k",
		"AudioChannels":      "2",
		"AudioSampleRate":    "48000",
		"BufferSize":         "4096k", // Doubled for anti-stuttering
		"MaxRate":            "30M",
		"Preset":             "superfast",
		"Tune":               "zerolatency",
		"ThreadQueueSize":    "2048", // Quadrupled for anti-stuttering
		"MaxMuxingQueueSize": "512",  // Doubled for anti-stuttering
		"Threads":            "8",    // Increased for better performance
		"Format":             "mpegts",
		"AnalyzeDuration":    "1000000", // Anti-stuttering input analysis
		"ProbeSize":          "1000000", // Anti-stuttering input analysis
		"FPSProbeSize":       "1",       // Anti-stuttering input analysis
		"FPSMode":            "cfr",     // Anti-stuttering A/V sync
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

	if config.AudioSampleRate != expectedFields["AudioSampleRate"] {
		t.Errorf("Expected AudioSampleRate to be %s, got %s", expectedFields["AudioSampleRate"], config.AudioSampleRate)
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

	// Check anti-stuttering fields
	if config.AnalyzeDuration != expectedFields["AnalyzeDuration"] {
		t.Errorf("Expected AnalyzeDuration to be %s, got %s", expectedFields["AnalyzeDuration"], config.AnalyzeDuration)
	}

	if config.ProbeSize != expectedFields["ProbeSize"] {
		t.Errorf("Expected ProbeSize to be %s, got %s", expectedFields["ProbeSize"], config.ProbeSize)
	}

	if config.FPSProbeSize != expectedFields["FPSProbeSize"] {
		t.Errorf("Expected FPSProbeSize to be %s, got %s", expectedFields["FPSProbeSize"], config.FPSProbeSize)
	}

	if config.FPSMode != expectedFields["FPSMode"] {
		t.Errorf("Expected FPSMode to be %s, got %s", expectedFields["FPSMode"], config.FPSMode)
	}
}

func TestBuildArgs(t *testing.T) {
	// Create a new config
	config := New()

	// Build the arguments
	args := config.BuildArgs()

	// Convert args to map for easier testing
	argMap := make(map[string]string)
	for i := 0; i < len(args)-1; i++ {
		if strings.HasPrefix(args[i], "-") {
			argMap[args[i]] = args[i+1]
		}
	}

	// Test essential flags are present (anti-stuttering values)
	essentialFlags := map[string]string{
		"-i":                     "pipe:0",
		"-c:v":                   "copy",
		"-c:a":                   "eac3",
		"-b:a":                   "384k",
		"-ac":                    "2",
		"-ar":                    "48000", // Anti-stuttering: audio sample rate
		"-bufsize":               "4096k", // Anti-stuttering: doubled buffer
		"-maxrate":               "30M",
		"-preset":                "superfast",
		"-tune":                  "zerolatency",
		"-max_muxing_queue_size": "512", // Anti-stuttering: doubled
		"-threads":               "8",   // Anti-stuttering: increased
		"-f":                     "mpegts",
		"-thread_queue_size":     "2048", // Anti-stuttering: quadrupled
		"-flush_packets":         "1",
		"-max_delay":             "0",
		"-analyzeduration":       "1000000", // Anti-stuttering: input analysis
		"-probesize":             "1000000", // Anti-stuttering: input analysis
		"-fpsprobesize":          "1",       // Anti-stuttering: input analysis
		"-fps_mode":              "cfr",     // Anti-stuttering: A/V sync
		"-err_detect":            "ignore_err",
		"-strict":                "experimental",
		"-skip_frame":            "nokey",
		"-avoid_negative_ts":     "make_zero",
	}

	for flag, expectedValue := range essentialFlags {
		if actualValue, exists := argMap[flag]; !exists {
			t.Errorf("Missing required flag: %s", flag)
		} else if actualValue != expectedValue {
			t.Errorf("Flag %s: expected %s, got %s", flag, expectedValue, actualValue)
		}
	}

	// Test that error resilience flags are present
	argsStr := strings.Join(args, " ")
	if !strings.Contains(argsStr, "+flush_packets+genpts+discardcorrupt") {
		t.Error("Missing AC4 error resilience flags in -fflags")
	}

	if !strings.Contains(argsStr, "-ignore_unknown") {
		t.Error("Missing -ignore_unknown flag for AC4 compatibility")
	}

	// Test output target is at the end
	if args[len(args)-1] != "pipe:1" {
		t.Errorf("Expected output target 'pipe:1' at end, got %s", args[len(args)-1])
	}

	// Test that we have a reasonable number of arguments (should be > 30 with error resilience)
	if len(args) < 30 {
		t.Errorf("Expected at least 30 arguments for enhanced config, got %d", len(args))
	}
}
