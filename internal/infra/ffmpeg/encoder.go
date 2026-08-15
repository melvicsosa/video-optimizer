package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// Encoder runs ffmpeg to produce the output described by a plan.
type Encoder struct {
	Bin string
}

// NewEncoder returns an Encoder bound to the ffmpeg binary in PATH.
func NewEncoder() Encoder { return Encoder{Bin: "ffmpeg"} }

// Encode writes the encoded file to dst, calling onProgress as ffmpeg reports.
func (e Encoder) Encode(ctx context.Context, plan domain.EncodePlan, dst string, onProgress func(domain.Progress)) error {
	args := BuildArgs(plan, dst)
	cmd := exec.CommandContext(ctx, e.Bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &tailWriter{buf: &stderr, limit: 8 << 10}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		readProgress(stdout, onProgress)
	}()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg failed: %w\n%s", err, lastLines(stderr.String(), 6))
	}
	return nil
}

// BuildArgs turns a plan into the ffmpeg argument list. Kept separate from the
// process handling so the recipe can be inspected and tested on its own.
func BuildArgs(plan domain.EncodePlan, dst string) []string {
	args := []string{
		"-hide_banner",
		"-nostdin",
		"-loglevel", "error",
		"-y",
		"-i", plan.Source.Path,
		"-progress", "pipe:1",
		"-nostats",
	}

	if plan.TargetHeight > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d:flags=lanczos", plan.TargetHeight))
	}

	crf := strconv.Itoa(plan.CRF)
	rate := strconv.FormatInt(plan.VideoBitrate, 10)
	buf := strconv.FormatInt(plan.VideoBitrate*2, 10)

	args = append(args, "-c:v", plan.Format.VideoEncoder)
	switch plan.Format.VideoEncoder {
	case "libvpx-vp9":
		// Constrained quality: CRF drives quality, -b:v caps the average bitrate.
		args = append(args,
			"-crf", crf,
			"-b:v", rate,
			"-deadline", "good",
			"-cpu-used", "3",
			"-row-mt", "1",
			"-tile-columns", "2",
			"-auto-alt-ref", "1",
			"-lag-in-frames", "25",
			"-g", "240",
		)
	case "libsvtav1":
		args = append(args,
			"-crf", crf,
			"-preset", "6",
			"-maxrate", rate,
			"-bufsize", buf,
			"-g", "240",
			"-svtav1-params", "tune=0",
		)
	case "libx265":
		args = append(args,
			"-crf", crf,
			"-maxrate", rate,
			"-bufsize", buf,
			"-preset", "medium",
			"-tag:v", "hvc1",
		)
	case "libx264":
		args = append(args,
			"-crf", crf,
			"-maxrate", rate,
			"-bufsize", buf,
			"-preset", "medium",
			"-profile:v", "high",
		)
	}
	args = append(args, "-pix_fmt", "yuv420p")

	if plan.AudioBitrate > 0 {
		args = append(args,
			"-c:a", plan.Format.AudioEncoder,
			"-b:a", strconv.FormatInt(plan.AudioBitrate, 10),
			"-ac", "2",
		)
	} else {
		args = append(args, "-an")
	}

	if plan.Format.Container == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}

	return append(args, dst)
}

// readProgress consumes the key=value stream produced by "-progress pipe:1".
func readProgress(r io.Reader, onProgress func(domain.Progress)) {
	scanner := bufio.NewScanner(r)
	var current domain.Progress
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us", "out_time_ms":
			// ffmpeg reports microseconds under both keys, out_time_ms included.
			if us, err := strconv.ParseInt(value, 10, 64); err == nil && us > 0 {
				current.Elapsed = time.Duration(us) * time.Microsecond
			}
		case "fps":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				current.FPS = f
			}
		case "bitrate":
			current.Bitrate = parseBitrate(value)
		case "speed":
			if s, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "x"), 64); err == nil {
				current.Speed = s
			}
		case "progress":
			if onProgress != nil {
				onProgress(current)
			}
		}
	}
}

func parseBitrate(v string) int64 {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "bits/s"))
	multiplier := 1.0
	switch {
	case strings.HasSuffix(v, "k"):
		multiplier, v = 1_000, strings.TrimSuffix(v, "k")
	case strings.HasSuffix(v, "M"):
		multiplier, v = 1_000_000, strings.TrimSuffix(v, "M")
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return int64(f * multiplier)
}

// tailWriter keeps only the last bytes written, enough to explain a failure
// without holding the whole ffmpeg log in memory.
type tailWriter struct {
	buf   *strings.Builder
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.buf.Len() > w.limit*2 {
		s := w.buf.String()
		w.buf.Reset()
		w.buf.WriteString(s[len(s)-w.limit:])
	}
	return len(p), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
