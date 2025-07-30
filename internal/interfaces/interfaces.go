// Package interfaces defines contracts for major components to enable dependency injection.
package interfaces

import (
	"context"
	"io"
	"net/http"
)

// HTTPClient defines the contract for HTTP client implementations.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (*http.Response, error)
}

// FFmpegConfig defines the contract for FFmpeg configuration.
type FFmpegConfig interface {
	BuildArgs() []string
	SetPreset(preset string)
	SetTune(tune string)
	SetAudioBitrate(bitrate string)
	SetAudioChannels(channels string)
}

// StreamHelper defines the contract for stream processing.
type StreamHelper interface {
	Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error)
	CopyWithActivityUpdate(ctx context.Context, dst io.Writer, src io.Reader, activityCallback func()) (int64, error)
}

// HDHRProxy defines the contract for HDHomeRun proxy implementations.
type HDHRProxy interface {
	FetchDeviceID() error
	DeviceID() string
	ReverseDeviceID() string
	CreateAPIHandler() http.Handler
	ProxyRequest(w http.ResponseWriter, r *http.Request)
	GetHDHRIP() string
}

// Transcoder defines the contract for transcoding implementations.
type Transcoder interface {
	TranscodeChannel(w http.ResponseWriter, r *http.Request, channel string) error
	DirectStreamChannel(w http.ResponseWriter, r *http.Request, channel string) error
	CreateMediaHandler() http.Handler
	StopAllTranscoding()
	Shutdown()
}

// ChannelInfo represents channel information from HDHomeRun.
type ChannelInfo struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
	HD          int    `json:"HD"`
	Favorite    int    `json:"Favorite"`
	AudioCodec  string `json:"AudioCodec"`
	VideoCodec  string `json:"VideoCodec"`
}

// SecurityValidator defines the contract for security validation.
type SecurityValidator interface {
	ValidateExecutable(path string) error
	ValidatePath(path string) error
	SanitizeInput(input string) string
}
