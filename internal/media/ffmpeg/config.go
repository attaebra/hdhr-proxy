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
}

// Ensure Config implements the FFmpegConfig interface.
var _ interfaces.FFmpegConfig = (*Config)(nil)

// NewOptimizedConfig returns an optimized configuration for streaming.
func NewOptimizedConfig() *Config {
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
	return []string{
		"-fflags", "+flush_packets", // Flush packets immediately for real-time streaming
		"-flush_packets", "1", // Enable packet flushing
		"-max_delay", "0", // Minimize delay for live streaming
		"-thread_queue_size", c.ThreadQueueSize,
		"-i", c.InputSource,
		"-c:v", c.VideoCodec,
		"-c:a", c.AudioCodec,
		"-b:a", c.AudioBitrate,
		"-ac", c.AudioChannels,
		"-bufsize", c.BufferSize,
		"-maxrate", c.MaxRate,
		"-preset", c.Preset,
		"-tune", c.Tune,
		"-max_muxing_queue_size", c.MaxMuxingQueueSize,
		"-threads", c.Threads,
		"-f", c.Format,
		c.OutputTarget,
	}
}
