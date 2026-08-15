package domain

import "time"

// Level identifies how aggressively a source should be compressed.
type Level string

// The levels accepted by the -preset flag, from safest to most aggressive.
const (
	LevelLight      Level = "light"      // keeps resolution, mild reduction
	LevelBalanced   Level = "balanced"   // keeps resolution, best trade-off
	LevelAggressive Level = "aggressive" // downscales to 720p, smallest output
)

// Preset is a compression intent expressed in business terms: how much weight
// we want to remove and whether we accept losing resolution to get there.
type Preset struct {
	Level     Level
	Name      string
	Summary   string
	Reduction float64 // share of the original size we aim to remove (0.25 = -25%)
	MaxHeight int     // 0 keeps the source height
	CRFAdjust int     // fine tuning applied on top of the codec base CRF
}

// Presets are the three choices offered to the user, ordered from safest to
// most aggressive.
var Presets = []Preset{
	{
		Level:     LevelLight,
		Name:      "Light",
		Summary:   "Keeps the source resolution. Visually identical, safest option.",
		Reduction: 0.25,
	},
	{
		Level:     LevelBalanced,
		Name:      "Balanced",
		Summary:   "Keeps the source resolution. Best size/quality trade-off for the web.",
		Reduction: 0.50,
	},
	{
		Level:     LevelAggressive,
		Name:      "Aggressive",
		Summary:   "Downscales to 720p. Smallest file, ideal for previews and mobile.",
		Reduction: 0.68,
		MaxHeight: 720,
	},
}

// PresetByLevel returns the preset matching the given level.
func PresetByLevel(level Level) (Preset, bool) {
	for _, p := range Presets {
		if p.Level == level {
			return p, true
		}
	}
	return Preset{}, false
}

// Format is an output recipe: a container plus the codecs used to fill it.
type Format struct {
	ID           string
	Name         string
	Container    string // file extension without the dot
	VideoEncoder string
	AudioEncoder string
	Note         string

	// BaseCRF is the constant-quality target per level. Scales differ per codec.
	BaseCRF map[Level]int
	// Efficiency expresses how many bits this codec needs compared to VP9 for
	// the same perceived quality. Lower is better.
	Efficiency float64
}

// Formats are the supported output recipes, ordered by recommendation.
var Formats = []Format{
	{
		ID:           "webm-vp9",
		Name:         "WebM · VP9",
		Container:    "webm",
		VideoEncoder: "libvpx-vp9",
		AudioEncoder: "libopus",
		Note:         "Universal browser support. The safe default for the web.",
		BaseCRF:      map[Level]int{LevelLight: 30, LevelBalanced: 33, LevelAggressive: 36},
		Efficiency:   1.0,
	},
	{
		ID:           "webm-av1",
		Name:         "WebM · AV1",
		Container:    "webm",
		VideoEncoder: "libsvtav1",
		AudioEncoder: "libopus",
		Note:         "Smallest files and faster to encode than VP9. Safari 17+.",
		BaseCRF:      map[Level]int{LevelLight: 30, LevelBalanced: 34, LevelAggressive: 38},
		Efficiency:   0.85,
	},
	{
		ID:           "mp4-h265",
		Name:         "MP4 · H.265",
		Container:    "mp4",
		VideoEncoder: "libx265",
		AudioEncoder: "aac",
		Note:         "Great on Apple devices. Not supported by Firefox.",
		BaseCRF:      map[Level]int{LevelLight: 24, LevelBalanced: 27, LevelAggressive: 30},
		Efficiency:   0.95,
	},
	{
		ID:           "mp4-h264",
		Name:         "MP4 · H.264",
		Container:    "mp4",
		VideoEncoder: "libx264",
		AudioEncoder: "aac",
		Note:         "Plays everywhere, including legacy devices. Largest output.",
		BaseCRF:      map[Level]int{LevelLight: 21, LevelBalanced: 24, LevelAggressive: 27},
		Efficiency:   1.3,
	},
}

// FormatByID returns the format matching the given id.
func FormatByID(id string) (Format, bool) {
	for _, f := range Formats {
		if f.ID == id {
			return f, true
		}
	}
	return Format{}, false
}

// DefaultFormat is the recipe used to estimate results before the user picks
// an explicit output format.
func DefaultFormat() Format { return Formats[0] }

// EncodePlan is the fully resolved recipe for one output file. Everything an
// Encoder needs lives here, so encoders stay dumb and the math stays testable.
type EncodePlan struct {
	Source Video
	Preset Preset
	Format Format

	CRF          int
	VideoBitrate int64 // ceiling, in bits per second
	AudioBitrate int64 // 0 when the source has no audio track
	TargetHeight int   // 0 keeps the source height
	TargetWidth  int   // informational, derived from the source aspect ratio

	// PredictedBitrate is the bitrate we expect the encoder to actually use,
	// available only when the source has been sampled.
	PredictedBitrate int64
	Measured         bool

	EstimatedBytes     int64
	EstimatedReduction float64
}

// bitrateBounds keeps the ceiling inside sane limits for a given output height,
// so a badly authored source cannot push us into unwatchable or wasteful territory.
func bitrateBounds(height int) (min, max int64) {
	switch {
	case height >= 2160:
		return 3_500_000, 20_000_000
	case height >= 1440:
		return 2_000_000, 12_000_000
	case height >= 1080:
		return 1_200_000, 8_000_000
	case height >= 720:
		return 800_000, 4_500_000
	default:
		return 350_000, 2_500_000
	}
}

func audioBitrateFor(f Format, level Level) int64 {
	opus := f.AudioEncoder == "libopus"
	switch level {
	case LevelLight:
		if opus {
			return 128_000
		}
		return 160_000
	case LevelAggressive:
		if opus {
			return 80_000
		}
		return 112_000
	default:
		if opus {
			return 96_000
		}
		return 128_000
	}
}

// BuildPlan resolves a source, a preset and a format into a concrete encode
// plan whose estimate is a guaranteed upper bound.
func BuildPlan(v Video, preset Preset, format Format) EncodePlan {
	return BuildPlanWith(v, preset, format, Complexity{})
}

// BuildPlanWith resolves a plan, refining the estimate with a complexity
// measurement when one is available.
//
// The bitrate ceiling is derived from the source: we ask for the reduction the
// preset promises, then correct it by how efficient the chosen codec is. The
// encoder runs in constrained-quality mode, so this ceiling is an upper bound:
// simple footage usually lands well below it, which is exactly what the
// complexity measurement lets us predict.
func BuildPlanWith(v Video, preset Preset, format Format, complexity Complexity) EncodePlan {
	info := v.Info

	targetHeight := 0
	outHeight := info.Height
	if preset.MaxHeight > 0 && info.Height > preset.MaxHeight {
		targetHeight = preset.MaxHeight
		outHeight = preset.MaxHeight
	}

	targetWidth := info.Width
	if targetHeight > 0 && info.Height > 0 {
		targetWidth = int(float64(info.Width) * float64(targetHeight) / float64(info.Height))
		targetWidth -= targetWidth % 2
	}

	audio := int64(0)
	if info.HasAudio {
		audio = audioBitrateFor(format, preset.Level)
		if info.AudioBitrate > 0 && info.AudioBitrate < audio {
			audio = info.AudioBitrate
		}
	}

	sourceVideo := info.EffectiveVideoBitrate()
	video := int64(float64(sourceVideo) * (1 - preset.Reduction) * format.Efficiency)

	minRate, maxRate := bitrateBounds(outHeight)
	if video < minRate {
		video = minRate
	}
	if video > maxRate {
		video = maxRate
	}
	// Never spend more bits than the source itself.
	if ceiling := int64(float64(sourceVideo) * 0.95); sourceVideo > 0 && video > ceiling {
		video = ceiling
	}

	crf := format.BaseCRF[preset.Level] + preset.CRFAdjust

	plan := EncodePlan{
		Source:       v,
		Preset:       preset,
		Format:       format,
		CRF:          crf,
		VideoBitrate: video,
		AudioBitrate: audio,
		TargetHeight: targetHeight,
		TargetWidth:  targetWidth,
	}
	// The encoder is quality driven, so the ceiling is only reached on complex
	// footage. When we measured the source we can predict where it will land.
	billed := video
	if predicted := complexity.Predict(crf, plan.OutputWidth()*plan.OutputHeight(), format.Efficiency); predicted > 0 {
		plan.PredictedBitrate = predicted
		plan.Measured = true
		if predicted < billed {
			billed = predicted
		}
	}

	plan.EstimatedBytes = estimateBytes(billed+audio, info.Duration)
	if info.SizeBytes > 0 && plan.EstimatedBytes > 0 {
		plan.EstimatedReduction = 1 - float64(plan.EstimatedBytes)/float64(info.SizeBytes)
	}
	return plan
}

// estimateBytes converts a bitrate ceiling and a duration into a file size,
// leaving a small margin for container overhead.
func estimateBytes(bitrate int64, d time.Duration) int64 {
	if bitrate <= 0 || d <= 0 {
		return 0
	}
	return int64(float64(bitrate) * d.Seconds() / 8 * 1.02)
}

// OutputHeight is the vertical resolution of the encoded file.
func (p EncodePlan) OutputHeight() int {
	if p.TargetHeight > 0 {
		return p.TargetHeight
	}
	return p.Source.Info.Height
}

// OutputWidth is the horizontal resolution of the encoded file.
func (p EncodePlan) OutputWidth() int {
	if p.TargetHeight > 0 {
		return p.TargetWidth
	}
	return p.Source.Info.Width
}
