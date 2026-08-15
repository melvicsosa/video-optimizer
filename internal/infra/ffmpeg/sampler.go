package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// Sampler measures source complexity by encoding short excerpts and weighing
// the result. It trades a few seconds of CPU for an estimate that reflects the
// actual footage instead of a worst case.
type Sampler struct {
	Bin string
	// Positions are the relative points of the timeline to sample.
	Positions []float64
	// Window is how much of the video each sample covers.
	Window time.Duration
	// CPUUsed speeds up the sampling encodes; faster settings spend slightly
	// more bits, which keeps the estimate on the conservative side.
	CPUUsed int
}

// NewSampler returns a Sampler with sensible defaults: three two second
// excerpts taken from the first, middle and last third of the video.
func NewSampler() Sampler {
	return Sampler{
		Bin:       "ffmpeg",
		Positions: []float64{0.15, 0.5, 0.8},
		Window:    2 * time.Second,
		CPUUsed:   5,
	}
}

// Sample encodes the excerpts described by plan and returns the measured
// complexity of the source. The excerpts are video only and use the plan's
// CRF, encoder and target resolution, so the measured bitrate can feed
// Complexity.Predict without further conversion. Sampling never touches the
// source or the output directory; everything happens in a temp dir.
func (s Sampler) Sample(ctx context.Context, plan domain.EncodePlan) (domain.Complexity, error) {
	duration := plan.Source.Info.Duration
	if duration <= 0 {
		return domain.Complexity{}, fmt.Errorf("cannot sample a source with unknown duration")
	}

	dir, err := os.MkdirTemp("", "vopt-sample")
	if err != nil {
		return domain.Complexity{}, err
	}
	defer os.RemoveAll(dir)

	window := s.Window
	if window <= 0 {
		window = 2 * time.Second
	}

	var (
		totalBytes   int64
		totalSeconds float64
		taken        int
	)
	for i, position := range s.Positions {
		at := time.Duration(float64(duration) * position)
		if at+window > duration {
			at = duration - window
		}
		if at < 0 {
			at = 0
		}
		covered := window
		if covered > duration {
			covered = duration
		}

		dst := filepath.Join(dir, fmt.Sprintf("sample-%d.webm", i))
		if err := s.encodeSample(ctx, plan, at, covered, dst); err != nil {
			return domain.Complexity{}, err
		}
		st, err := os.Stat(dst)
		if err != nil {
			return domain.Complexity{}, err
		}
		totalBytes += st.Size()
		totalSeconds += covered.Seconds()
		taken++

		// A short source is fully covered by the first excerpt.
		if covered >= duration {
			break
		}
	}

	if totalSeconds <= 0 || totalBytes <= 0 {
		return domain.Complexity{}, fmt.Errorf("sampling produced no data")
	}

	return domain.Complexity{
		RefBitrate:    int64(float64(totalBytes*8) / totalSeconds),
		RefCRF:        plan.CRF,
		RefPixels:     plan.OutputWidth() * plan.OutputHeight(),
		RefEfficiency: plan.Format.Efficiency,
		Samples:       taken,
	}, nil
}

func (s Sampler) encodeSample(ctx context.Context, plan domain.EncodePlan, at, window time.Duration, dst string) error {
	cpuUsed := s.CPUUsed
	if cpuUsed == 0 {
		cpuUsed = 5
	}

	args := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-ss", formatSeconds(at),
		"-t", formatSeconds(window),
		"-i", plan.Source.Path,
	}
	if plan.TargetHeight > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d:flags=bilinear", plan.TargetHeight))
	}
	args = append(args,
		"-an",
		"-c:v", plan.Format.VideoEncoder,
		"-crf", strconv.Itoa(plan.CRF),
	)
	switch plan.Format.VideoEncoder {
	case "libvpx-vp9":
		args = append(args,
			"-b:v", strconv.FormatInt(plan.VideoBitrate, 10),
			"-deadline", "good",
			"-cpu-used", strconv.Itoa(cpuUsed),
			"-row-mt", "1",
		)
	case "libsvtav1":
		args = append(args, "-preset", "9")
	default:
		args = append(args, "-preset", "veryfast")
	}
	args = append(args, "-pix_fmt", "yuv420p", dst)

	out, err := exec.CommandContext(ctx, s.Bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sample encode: %w\n%s", err, lastLines(string(out), 4))
	}
	return nil
}
