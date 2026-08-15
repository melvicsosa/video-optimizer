package domain

import "testing"

func TestComplexityPredict(t *testing.T) {
	base := Complexity{
		RefBitrate:    2_000_000,
		RefCRF:        33,
		RefPixels:     1920 * 1080,
		RefEfficiency: 1.0,
		Samples:       3,
	}

	tests := []struct {
		name       string
		crf        int
		pixels     int
		efficiency float64
		wantMin    int64
		wantMax    int64
	}{
		{name: "same settings reproduce the measurement", crf: 33, pixels: 1920 * 1080, efficiency: 1.0, wantMin: 1_950_000, wantMax: 2_050_000},
		{name: "six crf steps up roughly halve the bitrate", crf: 39, pixels: 1920 * 1080, efficiency: 1.0, wantMin: 950_000, wantMax: 1_050_000},
		{name: "six crf steps down roughly double it", crf: 27, pixels: 1920 * 1080, efficiency: 1.0, wantMin: 3_900_000, wantMax: 4_100_000},
		{name: "downscaling to 720p costs fewer bits", crf: 33, pixels: 1280 * 720, efficiency: 1.0, wantMin: 1_000_000, wantMax: 1_300_000},
		{name: "a more efficient codec needs fewer bits", crf: 33, pixels: 1920 * 1080, efficiency: 0.85, wantMin: 1_650_000, wantMax: 1_750_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base.Predict(tt.crf, tt.pixels, tt.efficiency)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("predicted %d bps, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestComplexityInvalidMeasurementPredictsNothing(t *testing.T) {
	var empty Complexity

	if empty.Valid() {
		t.Error("a zero complexity must not be valid")
	}
	if got := empty.Predict(30, 1920*1080, 1.0); got != 0 {
		t.Errorf("predicted %d, want 0 without a measurement", got)
	}
}
