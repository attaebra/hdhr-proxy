package ffmpeg

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

// NewOptimizedConfig returns an optimized configuration for streaming.
func NewOptimizedConfig() *Config {
	return &Config{
		InputSource:        "pipe:0",
		OutputTarget:       "pipe:1",
		VideoCodec:         "copy",
		AudioCodec:         "eac3",
		AudioBitrate:       "384k",
		AudioChannels:      "2",
		BufferSize:         "12288k", // Increased from 8192k
		MaxRate:            "30M",    // Increased from 20M
		Preset:             "superfast",
		Tune:               "zerolatency", // Added for streaming
		ThreadQueueSize:    "4096",        // Added to prevent underruns
		MaxMuxingQueueSize: "1024",
		Threads:            "4", // Increased from 2
		Format:             "mpegts",
	}
}

// BuildArgs constructs command line arguments for FFmpeg.
func (c *Config) BuildArgs() []string {
	return []string{
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
