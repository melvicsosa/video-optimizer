package domain

import "math"

// Complexity is the outcome of encoding a few short samples of a source with a
// known reference setting. It turns "how heavy is this footage" into a number,
// which lets us predict output sizes instead of only promising an upper bound.
//
// A static talking head and a handheld action shot answer the bitrate ceiling
// very differently; sampling is the only cheap way to tell them apart.
type Complexity struct {
	RefBitrate    int64   // video bits per second measured on the samples
	RefCRF        int     // CRF used while sampling
	RefPixels     int     // pixels per frame while sampling
	RefEfficiency float64 // efficiency of the codec used while sampling
	Samples       int
}

// Valid reports whether the measurement can be used for predictions.
func (c Complexity) Valid() bool {
	return c.RefBitrate > 0 && c.RefPixels > 0 && c.RefEfficiency > 0
}

// Predict estimates the video bitrate the given settings would produce.
//
// Two well established rules of thumb drive it: bitrate roughly halves every
// six CRF steps, and it grows sublinearly with the pixel count.
func (c Complexity) Predict(crf, pixels int, efficiency float64) int64 {
	if !c.Valid() || pixels <= 0 {
		return 0
	}
	bitrate := float64(c.RefBitrate)
	bitrate *= math.Pow(2, float64(c.RefCRF-crf)/6)
	bitrate *= math.Pow(float64(pixels)/float64(c.RefPixels), 0.75)
	bitrate *= efficiency / c.RefEfficiency
	if bitrate < 1 {
		return 0
	}
	return int64(bitrate)
}

// SamplePlan returns the plan used to measure a source: the balanced preset on
// the default format, at the source resolution.
func SamplePlan(v Video) EncodePlan {
	preset, _ := PresetByLevel(LevelBalanced)
	return BuildPlan(v, preset, DefaultFormat())
}
