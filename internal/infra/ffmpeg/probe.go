// Package ffmpeg adapts the ffmpeg/ffprobe binaries to the domain ports.
package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// Prober reads media metadata using ffprobe.
type Prober struct {
	Bin string
}

// NewProber returns a Prober bound to the ffprobe binary in PATH.
func NewProber() Prober { return Prober{Bin: "ffprobe"} }

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecName    string `json:"codec_name"`
		CodecType    string `json:"codec_type"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
		BitRate      string `json:"bit_rate"`
		Duration     string `json:"duration"`
	} `json:"streams"`
}

// Probe returns the technical description of the file at path.
func (p Prober) Probe(ctx context.Context, path string) (domain.MediaInfo, error) {
	cmd := exec.CommandContext(ctx, p.Bin,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return domain.MediaInfo{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var parsed probeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return domain.MediaInfo{}, fmt.Errorf("decode ffprobe output: %w", err)
	}

	info := domain.MediaInfo{
		Duration:  parseSeconds(parsed.Format.Duration),
		SizeBytes: parseInt(parsed.Format.Size),
	}
	if info.SizeBytes == 0 {
		if st, err := os.Stat(path); err == nil {
			info.SizeBytes = st.Size()
		}
	}

	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			if info.VideoCodec != "" {
				continue // keep the first video stream, ignore cover art
			}
			info.VideoCodec = s.CodecName
			info.Width = s.Width
			info.Height = s.Height
			info.FPS = parseRate(s.AvgFrameRate, s.RFrameRate)
			info.VideoBitrate = parseInt(s.BitRate)
			if info.Duration == 0 {
				info.Duration = parseSeconds(s.Duration)
			}
		case "audio":
			if info.HasAudio {
				continue
			}
			info.HasAudio = true
			info.AudioCodec = s.CodecName
			info.AudioBitrate = parseInt(s.BitRate)
		}
	}

	if info.Width == 0 || info.Height == 0 {
		return info, fmt.Errorf("%s has no usable video stream", path)
	}
	return info, nil
}

func parseSeconds(v string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

func parseInt(v string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseRate reads ffprobe's "30000/1001" style fractions, trying each candidate
// in order and returning the first usable one.
func parseRate(candidates ...string) float64 {
	for _, c := range candidates {
		num, den, ok := strings.Cut(strings.TrimSpace(c), "/")
		if !ok {
			continue
		}
		n, err1 := strconv.ParseFloat(num, 64)
		d, err2 := strconv.ParseFloat(den, 64)
		if err1 != nil || err2 != nil || d == 0 || n == 0 {
			continue
		}
		return n / d
	}
	return 0
}
