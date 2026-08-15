// Package domain holds the business rules of the optimizer: what a video is,
// how a compression plan is derived and what the expected outcome looks like.
// It knows nothing about ffmpeg, the terminal or the filesystem.
package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Video is a candidate file discovered in the working directory.
type Video struct {
	Path      string
	SizeBytes int64
	Info      MediaInfo
	Probed    bool
	ProbeErr  error
}

// FileName returns the file name including its extension.
func (v Video) FileName() string { return filepath.Base(v.Path) }

// BaseName returns the file name without its extension.
func (v Video) BaseName() string {
	name := v.FileName()
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// MediaInfo is the technical description of a video file as reported by a Prober.
type MediaInfo struct {
	Duration     time.Duration
	Width        int
	Height       int
	FPS          float64
	VideoCodec   string
	AudioCodec   string
	VideoBitrate int64 // bits per second
	AudioBitrate int64 // bits per second
	SizeBytes    int64
	HasAudio     bool
}

// TotalBitrate is the overall bitrate of the file, derived from its size when
// the container does not report one.
func (m MediaInfo) TotalBitrate() int64 {
	if m.Duration <= 0 {
		return 0
	}
	return int64(float64(m.SizeBytes*8) / m.Duration.Seconds())
}

// EffectiveVideoBitrate falls back to size-derived numbers when the video
// stream carries no bitrate metadata (common in Matroska/WebM sources).
func (m MediaInfo) EffectiveVideoBitrate() int64 {
	if m.VideoBitrate > 0 {
		return m.VideoBitrate
	}
	total := m.TotalBitrate()
	if total <= 0 {
		return 0
	}
	audio := m.AudioBitrate
	if audio <= 0 && m.HasAudio {
		audio = 128_000
	}
	if video := total - audio; video > 0 {
		return video
	}
	return total
}

// Resolution renders the frame size, e.g. "1920x1080".
func (m MediaInfo) Resolution() string {
	return fmt.Sprintf("%dx%d", m.Width, m.Height)
}

// ResolutionLabel renders the vertical resolution as a marketing-style label,
// e.g. "1080p".
func (m MediaInfo) ResolutionLabel() string {
	switch {
	case m.Height >= 2160:
		return "4K"
	case m.Height >= 1440:
		return "1440p"
	case m.Height >= 1080:
		return "1080p"
	case m.Height >= 720:
		return "720p"
	case m.Height >= 480:
		return "480p"
	default:
		return fmt.Sprintf("%dp", m.Height)
	}
}
