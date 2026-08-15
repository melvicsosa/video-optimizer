package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// ThumbGrabber extracts still frames using ffmpeg.
type ThumbGrabber struct {
	Bin string
	// Quality is the JPEG quality scale, 2 (best) to 31 (worst).
	Quality int
}

// NewThumbGrabber returns a grabber bound to the ffmpeg binary in PATH.
func NewThumbGrabber() ThumbGrabber { return ThumbGrabber{Bin: "ffmpeg", Quality: 2} }

// Grab writes the frame at the given position to dst. A height of 0 keeps the
// source resolution.
func (t ThumbGrabber) Grab(ctx context.Context, src, dst string, at time.Duration, height int) error {
	quality := t.Quality
	if quality == 0 {
		quality = 2
	}

	args := []string{
		"-hide_banner",
		"-nostdin",
		"-loglevel", "error",
		"-y",
		"-ss", formatSeconds(at),
		"-i", src,
		"-frames:v", "1",
	}
	if height > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d:flags=lanczos", height))
	}
	args = append(args, "-q:v", strconv.Itoa(quality), dst)

	out, err := exec.CommandContext(ctx, t.Bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("thumbnail at %s: %w\n%s", formatSeconds(at), err, lastLines(string(out), 4))
	}
	return nil
}

func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
