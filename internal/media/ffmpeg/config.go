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
		BufferSize:         "12288k",
		MaxRate:            "30M",
		Preset:             "superfast",
		Tune:               "zerolatency",
		ThreadQueueSize:    "4096",
		MaxMuxingQueueSize: "1024",
		Threads:            "4",
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
