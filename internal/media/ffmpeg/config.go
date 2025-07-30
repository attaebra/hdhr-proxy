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
	AudioCodec    string
	AudioBitrate  string
	AudioChannels string

	// Buffer and streaming settings
	BufferSize         string
	MaxRate            string
	Preset             string
	Tune               string
	ThreadQueueSize    string
	MaxMuxingQueueSize string
	Threads            string
	Format             string

	// Error resilience settings
	ErrorDetection   string
	SkipFrame        string
	StrictLevel      string
	ReconnectOptions bool
}

// Ensure Config implements the Config interface.
var _ interfaces.Config = (*Config)(nil)

// New returns a configuration for AC4 streaming with error resilience.
func New() *Config {
	return &Config{
		InputSource:        "pipe:0",
		OutputTarget:       "pipe:1",
		VideoCodec:         "copy",
		AudioCodec:         "eac3",
		AudioBitrate:       "384k",
		AudioChannels:      "2",
		BufferSize:         "2048k",
		MaxRate:            "30M",
		Preset:             "superfast",
		Tune:               "zerolatency",
		ThreadQueueSize:    "512",
		MaxMuxingQueueSize: "256",
		Threads:            "4",
		Format:             "mpegts",

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

// SetAudioChannels sets the number of audio channels.
func (c *Config) SetAudioChannels(channels string) {
	c.AudioChannels = channels
}

// BuildArgs constructs command line arguments for FFmpeg.
func (c *Config) BuildArgs() []string {
	args := []string{
		// Input flags for error resilience
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
		"-avoid_negative_ts", "make_zero", // Handle timestamp issues

		// Performance settings
		"-bufsize", c.BufferSize,
		"-maxrate", c.MaxRate,
		"-preset", c.Preset,
		"-tune", c.Tune,
		"-max_muxing_queue_size", c.MaxMuxingQueueSize,
		"-threads", c.Threads,

		// Output format
		"-f", c.Format,
		c.OutputTarget,
	}

	// Note: ReconnectOptions are reserved for future network input enhancement
	// Currently using pipes, so no additional reconnection flags needed

	return args
}
