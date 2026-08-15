package domain

import (
	"testing"
	"time"
)

// source1080p is a five minute, heavily over-encoded 1080p clip: the shape of
// file this tool exists for.
func source1080p() Video {
	info := MediaInfo{
		Duration:     5 * time.Minute,
		Width:        1920,
		Height:       1080,
		FPS:          30,
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		VideoBitrate: 9_500_000,
		AudioBitrate: 128_000,
		SizeBytes:    360_000_000,
		HasAudio:     true,
	}
	return Video{Path: "clip.mp4", SizeBytes: info.SizeBytes, Info: info, Probed: true}
}

func TestBuildPlanReduction(t *testing.T) {
	tests := []struct {
		name          string
		level         Level
		wantHeight    int
		minReduction  float64
		maxBitrateBPS int64
	}{
		{name: "light keeps resolution", level: LevelLight, wantHeight: 1080, minReduction: 0.20, maxBitrateBPS: 8_000_000},
		{name: "balanced keeps resolution", level: LevelBalanced, wantHeight: 1080, minReduction: 0.45, maxBitrateBPS: 8_000_000},
		{name: "aggressive downscales to 720p", level: LevelAggressive, wantHeight: 720, minReduction: 0.65, maxBitrateBPS: 4_500_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, ok := PresetByLevel(tt.level)
			if !ok {
				t.Fatalf("unknown level %q", tt.level)
			}

			plan := BuildPlan(source1080p(), preset, DefaultFormat())

			if got := plan.OutputHeight(); got != tt.wantHeight {
				t.Errorf("output height = %d, want %d", got, tt.wantHeight)
			}
			if plan.EstimatedReduction < tt.minReduction {
				t.Errorf("reduction = %.2f, want at least %.2f", plan.EstimatedReduction, tt.minReduction)
			}
			if plan.VideoBitrate > tt.maxBitrateBPS {
				t.Errorf("bitrate ceiling = %d, want at most %d", plan.VideoBitrate, tt.maxBitrateBPS)
			}
			if plan.Measured {
				t.Error("plan should not claim to be measured without a complexity sample")
			}
		})
	}
}

func TestBuildPlanKeepsEvenWidth(t *testing.T) {
	video := source1080p()
	video.Info.Width = 1919 // odd width, yuv420p needs even dimensions

	preset, _ := PresetByLevel(LevelAggressive)
	plan := BuildPlan(video, preset, DefaultFormat())

	if plan.OutputWidth()%2 != 0 {
		t.Errorf("output width = %d, want an even number", plan.OutputWidth())
	}
}

func TestBuildPlanNeverExceedsSourceBitrate(t *testing.T) {
	video := source1080p()
	video.Info.VideoBitrate = 900_000 // already tightly encoded

	preset, _ := PresetByLevel(LevelLight)
	plan := BuildPlan(video, preset, DefaultFormat())

	if plan.VideoBitrate > video.Info.VideoBitrate {
		t.Errorf("bitrate = %d, want at most the source bitrate %d", plan.VideoBitrate, video.Info.VideoBitrate)
	}
}

func TestBuildPlanWithoutAudio(t *testing.T) {
	video := source1080p()
	video.Info.HasAudio = false
	video.Info.AudioBitrate = 0

	preset, _ := PresetByLevel(LevelBalanced)
	plan := BuildPlan(video, preset, DefaultFormat())

	if plan.AudioBitrate != 0 {
		t.Errorf("audio bitrate = %d, want 0 for a silent source", plan.AudioBitrate)
	}
}

func TestBuildPlanWithMeasurementPredictsSmallerOutput(t *testing.T) {
	video := source1080p()
	preset, _ := PresetByLevel(LevelBalanced)

	// Simple footage: the samples came out far below the ceiling.
	complexity := Complexity{
		RefBitrate:    600_000,
		RefCRF:        33,
		RefPixels:     1920 * 1080,
		RefEfficiency: 1.0,
		Samples:       3,
	}

	bound := BuildPlan(video, preset, DefaultFormat())
	measured := BuildPlanWith(video, preset, DefaultFormat(), complexity)

	if !measured.Measured {
		t.Fatal("plan should be flagged as measured")
	}
	if measured.EstimatedBytes >= bound.EstimatedBytes {
		t.Errorf("measured estimate %d should be smaller than the upper bound %d",
			measured.EstimatedBytes, bound.EstimatedBytes)
	}
	if measured.VideoBitrate != bound.VideoBitrate {
		t.Error("measuring must refine the estimate, not the encoder ceiling")
	}
}

func TestBuildPlanIgnoresMeasurementAboveCeiling(t *testing.T) {
	video := source1080p()
	preset, _ := PresetByLevel(LevelAggressive)

	// Very demanding footage: the prediction sits above what we allow.
	complexity := Complexity{
		RefBitrate:    30_000_000,
		RefCRF:        33,
		RefPixels:     1920 * 1080,
		RefEfficiency: 1.0,
		Samples:       3,
	}

	plan := BuildPlanWith(video, preset, DefaultFormat(), complexity)
	bound := BuildPlan(video, preset, DefaultFormat())

	if plan.EstimatedBytes != bound.EstimatedBytes {
		t.Errorf("estimate = %d, want the ceiling based %d when the prediction is higher",
			plan.EstimatedBytes, bound.EstimatedBytes)
	}
}

func TestBuildPlanDerivesBitrateFromSize(t *testing.T) {
	video := source1080p()
	video.Info.VideoBitrate = 0 // container reported nothing

	preset, _ := PresetByLevel(LevelBalanced)
	plan := BuildPlan(video, preset, DefaultFormat())

	if plan.VideoBitrate <= 0 {
		t.Fatal("bitrate should be derived from size and duration")
	}
	if plan.EstimatedBytes <= 0 || plan.EstimatedBytes >= video.SizeBytes {
		t.Errorf("estimate = %d, want a positive value below the source size", plan.EstimatedBytes)
	}
}
