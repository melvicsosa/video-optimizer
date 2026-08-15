package ffmpeg

import (
	"strings"
	"testing"
	"time"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

func testPlan(t *testing.T, level domain.Level, formatID string) domain.EncodePlan {
	t.Helper()

	info := domain.MediaInfo{
		Duration:     2 * time.Minute,
		Width:        1920,
		Height:       1080,
		FPS:          30,
		VideoBitrate: 9_000_000,
		AudioBitrate: 128_000,
		SizeBytes:    135_000_000,
		HasAudio:     true,
	}
	video := domain.Video{Path: "in.mp4", SizeBytes: info.SizeBytes, Info: info, Probed: true}

	preset, ok := domain.PresetByLevel(level)
	if !ok {
		t.Fatalf("unknown level %q", level)
	}
	format, ok := domain.FormatByID(formatID)
	if !ok {
		t.Fatalf("unknown format %q", formatID)
	}
	return domain.BuildPlan(video, preset, format)
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		level    domain.Level
		formatID string
		contains []string
		absent   []string
	}{
		{
			name:     "vp9 uses constrained quality",
			level:    domain.LevelBalanced,
			formatID: "webm-vp9",
			contains: []string{"-c:v", "libvpx-vp9", "-crf", "-b:v", "-row-mt", "-c:a", "libopus"},
			absent:   []string{"-movflags", "-an"},
		},
		{
			name:     "aggressive scales down",
			level:    domain.LevelAggressive,
			formatID: "webm-vp9",
			contains: []string{"-vf", "scale=-2:720:flags=lanczos"},
		},
		{
			name:     "av1 caps the bitrate with maxrate",
			level:    domain.LevelBalanced,
			formatID: "webm-av1",
			contains: []string{"libsvtav1", "-maxrate", "-bufsize", "-preset"},
		},
		{
			name:     "mp4 output is streamable",
			level:    domain.LevelBalanced,
			formatID: "mp4-h265",
			contains: []string{"libx265", "-movflags", "+faststart", "-c:a", "aac"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := strings.Join(BuildArgs(testPlan(t, tt.level, tt.formatID), "out.webm"), " ")

			for _, want := range tt.contains {
				if !strings.Contains(args, want) {
					t.Errorf("args missing %q\ngot: %s", want, args)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(args, unwanted) {
					t.Errorf("args should not contain %q\ngot: %s", unwanted, args)
				}
			}
		})
	}
}

func TestBuildArgsSilentSourceDropsAudio(t *testing.T) {
	plan := testPlan(t, domain.LevelBalanced, "webm-vp9")
	plan.AudioBitrate = 0

	args := strings.Join(BuildArgs(plan, "out.webm"), " ")

	if !strings.Contains(args, "-an") {
		t.Errorf("expected -an for a silent source\ngot: %s", args)
	}
	if strings.Contains(args, "libopus") {
		t.Errorf("expected no audio encoder for a silent source\ngot: %s", args)
	}
}

func TestBuildArgsPutsDestinationLast(t *testing.T) {
	args := BuildArgs(testPlan(t, domain.LevelLight, "webm-vp9"), "out.webm")

	if got := args[len(args)-1]; got != "out.webm" {
		t.Errorf("last argument = %q, want the destination", got)
	}
}

func TestReadProgress(t *testing.T) {
	stream := strings.Join([]string{
		"frame=120",
		"fps=48.5",
		"bitrate=1536.2kbits/s",
		"out_time_ms=4000000",
		"speed=1.62x",
		"progress=continue",
		"out_time_ms=8000000",
		"speed=1.70x",
		"progress=end",
	}, "\n")

	var updates []domain.Progress
	readProgress(strings.NewReader(stream), func(p domain.Progress) {
		updates = append(updates, p)
	})

	if len(updates) != 2 {
		t.Fatalf("got %d progress updates, want 2", len(updates))
	}
	if updates[0].Elapsed != 4*time.Second {
		t.Errorf("first elapsed = %s, want 4s", updates[0].Elapsed)
	}
	if updates[0].Speed != 1.62 {
		t.Errorf("first speed = %v, want 1.62", updates[0].Speed)
	}
	if updates[0].FPS != 48.5 {
		t.Errorf("first fps = %v, want 48.5", updates[0].FPS)
	}
	if updates[0].Bitrate != 1_536_200 {
		t.Errorf("first bitrate = %d, want 1536200", updates[0].Bitrate)
	}
	if updates[1].Elapsed != 8*time.Second {
		t.Errorf("second elapsed = %s, want 8s", updates[1].Elapsed)
	}
}

func TestReadProgressIgnoresGarbage(t *testing.T) {
	var calls int
	readProgress(strings.NewReader("nonsense\n\nout_time_ms=not-a-number\nprogress=end\n"), func(domain.Progress) {
		calls++
	})

	if calls != 1 {
		t.Errorf("got %d updates, want 1", calls)
	}
}
