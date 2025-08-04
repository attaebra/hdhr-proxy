package ffmpeg

import "github.com/attaebra/hdhr-proxy/internal/interfaces"

// Config contains optimized FFmpeg parameters.
type Config struct {
	// Input/output configuration
	InputSource  string
	OutputTarget string

	// Video settings
	VideoCodec string

	// Audio settings
	AudioCodec      string
	AudioProfile    string
	AudioBitrate    string
	AudioChannels   string
	AudioSampleRate string

	// Buffer and streaming settings
	BufferSize         string
	MaxRate            string
	Preset             string
	Tune               string
	ThreadQueueSize    string
	MaxMuxingQueueSize string
	Threads            string
	Format             string

	// Input analysis settings (anti-stuttering)
	AnalyzeDuration string
	ProbeSize       string
	FPSProbeSize    string
	FPSMode         string

	// Error resilience settings
	ErrorDetection   string
	SkipFrame        string
	StrictLevel      string
	ReconnectOptions bool
}

// Ensure Config implements the Config interface.
var _ interfaces.Config = (*Config)(nil)

// New returns a configuration for AC4 streaming with error resilience and anti-stuttering improvements.
func New() *Config {
	return &Config{
		InputSource:     "pipe:0",
		OutputTarget:    "pipe:1",
		VideoCodec:      "copy",
		AudioCodec:      "eac3",
		AudioBitrate:    "384k",
		AudioChannels:   "2",
		AudioSampleRate: "48000", // Fixed sample rate for audio stability

		// Anti-stuttering buffer improvements
		BufferSize:         "4096k", // Doubled from 2048k for better buffering
		MaxRate:            "30M",
		Preset:             "superfast",
		Tune:               "zerolatency",
		ThreadQueueSize:    "2048", // Quadrupled from 512 for AC4 streams
		MaxMuxingQueueSize: "512",  // Doubled from 256 for throughput
		Threads:            "2",    // Conservative for shared server (4 streams × 2 = 8 threads max)
		Format:             "mpegts",

		// Input analysis for faster startup (anti-stuttering)
		AnalyzeDuration: "1000000", // 1 second limit for faster AC4 analysis
		ProbeSize:       "1000000", // 1MB limit to prevent analysis hanging
		FPSProbeSize:    "1",       // Immediate frame rate detection
		FPSMode:         "cfr",     // Constant frame rate for A/V sync

		// Error resilience for AC4 streams
		ErrorDetection:   "ignore_err",   // Ignore decoding errors instead of crashing
		SkipFrame:        "nokey",        // Skip corrupted frames, keep keyframes
		StrictLevel:      "experimental", // Allow experimental AC4 codec
		ReconnectOptions: true,           // Enable reconnection on stream errors
	}
}

// SetPreset sets the FFmpeg preset.
func (c *Config) SetPreset(preset string) {
	c.Preset = preset
}

// SetTune sets the FFmpeg tune option.
func (c *Config) SetTune(tune string) {
	c.Tune = tune
}

// SetAudioBitrate sets the audio bitrate.
func (c *Config) SetAudioBitrate(bitrate string) {
	c.AudioBitrate = bitrate
}

// SetAudioProfile sets the audio profile.
func (c *Config) SetAudioProfile(profile string) {
	c.AudioProfile = profile
}

// SetAudioChannels sets the number of audio channels.
func (c *Config) SetAudioChannels(channels string) {
	c.AudioChannels = channels
}

// BuildArgs constructs command line arguments for FFmpeg with anti-stuttering improvements.
func (c *Config) BuildArgs() []string {
	args := []string{}

	// Input analysis flags for faster startup (anti-stuttering)
	if c.AnalyzeDuration != "" {
		args = append(args, "-analyzeduration", c.AnalyzeDuration)
	}
	if c.ProbeSize != "" {
		args = append(args, "-probesize", c.ProbeSize)
	}
	if c.FPSProbeSize != "" {
		args = append(args, "-fpsprobesize", c.FPSProbeSize)
	}

	// Input flags for error resilience
	args = append(args,
		"-fflags", "+flush_packets+genpts+discardcorrupt", // Generate PTS, discard corrupted packets
		"-flush_packets", "1", // Enable packet flushing
		"-max_delay", "0", // Minimize delay for live streaming
		"-err_detect", c.ErrorDetection, // Handle decoding errors gracefully
		"-ignore_unknown",          // Ignore unknown stream types
		"-skip_frame", c.SkipFrame, // Skip corrupted frames but keep stream alive
		"-strict", c.StrictLevel, // Allow experimental codecs (AC4)
		"-thread_queue_size", c.ThreadQueueSize,

		// Input source
		"-i", c.InputSource,

		// Video codec (copy - no re-encoding)
		"-c:v", c.VideoCodec,

		// Audio codec settings with error recovery
		"-c:a", c.AudioCodec,
		"-b:a", c.AudioBitrate,
		"-ac", c.AudioChannels,
	)

	// Add audio profile if specified
	if c.AudioProfile != "" {
		args = append(args, "-profile:a", c.AudioProfile)
	}

	// Add audio sample rate for stability
	if c.AudioSampleRate != "" {
		args = append(args, "-ar", c.AudioSampleRate)
	}

	// Timestamp handling
	args = append(args, "-avoid_negative_ts", "make_zero")

	// Add frame rate mode for A/V sync (anti-stuttering)
	if c.FPSMode != "" {
		args = append(args, "-fps_mode", c.FPSMode)
	}

	// Performance settings
	args = append(args,
		"-bufsize", c.BufferSize,
		"-maxrate", c.MaxRate,
		"-preset", c.Preset,
		"-tune", c.Tune,
		"-max_muxing_queue_size", c.MaxMuxingQueueSize,
		"-threads", c.Threads,

		// Output format
		"-f", c.Format,
		c.OutputTarget,
	)

	// Note: ReconnectOptions are reserved for future network input enhancement
	// Currently using pipes, so no additional reconnection flags needed

	return args
}
