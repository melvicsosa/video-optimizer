// Package app wires the domain rules to the adapters: it runs the optimization
// pipeline (encode, thumbnail, report) and knows nothing about the UI.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// Optimizer executes compression plans and writes the resulting artifacts.
type Optimizer struct {
	Encoder domain.Encoder
	Thumbs  domain.ThumbGrabber
	OutDir  string
	// KeepNames writes outputs using the original file name instead of a slug.
	KeepNames bool
	// Report writes a JSON sidecar describing the output.
	Report bool
}

// Result describes what the pipeline produced.
type Result struct {
	Plan        domain.EncodePlan
	VideoPath   string
	ThumbPath   string
	ReportPath  string
	SourceBytes int64
	OutputBytes int64
	Reduction   float64
	Took        time.Duration
}

// Run encodes the plan, captures the poster frame when the spec asks for one
// and writes the optional JSON report. Progress callbacks come straight from
// the encoder.
func (o Optimizer) Run(ctx context.Context, plan domain.EncodePlan, thumb domain.ThumbSpec, onProgress func(domain.Progress)) (Result, error) {
	started := time.Now()

	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}

	base := plan.Source.BaseName()
	if !o.KeepNames {
		base = Slug(base)
	}

	videoPath := filepath.Join(o.OutDir, base+"."+plan.Format.Container)

	if err := o.Encoder.Encode(ctx, plan, videoPath, onProgress); err != nil {
		return Result{}, err
	}

	thumbAt := thumb.For(plan.Source.Info.Duration)
	thumbPath := ""
	if thumb.Enabled {
		thumbPath = filepath.Join(o.OutDir, base+"-thumb.jpg")
		if err := o.Thumbs.Grab(ctx, plan.Source.Path, thumbPath, thumbAt, plan.OutputHeight()); err != nil {
			return Result{}, err
		}
	}

	res := Result{
		Plan:        plan,
		VideoPath:   videoPath,
		ThumbPath:   thumbPath,
		SourceBytes: plan.Source.SizeBytes,
		Took:        time.Since(started),
	}
	if st, err := os.Stat(videoPath); err == nil {
		res.OutputBytes = st.Size()
	}
	if res.SourceBytes > 0 && res.OutputBytes > 0 {
		res.Reduction = 1 - float64(res.OutputBytes)/float64(res.SourceBytes)
	}

	if o.Report {
		path := filepath.Join(o.OutDir, base+".json")
		if err := writeReport(path, res, thumb, thumbAt); err != nil {
			return res, fmt.Errorf("write report: %w", err)
		}
		res.ReportPath = path
	}
	return res, nil
}

type report struct {
	Source      string  `json:"source"`
	Video       string  `json:"video"`
	Thumbnail   string  `json:"thumbnail,omitempty"`
	ThumbnailAt float64 `json:"thumbnailAtSeconds,omitempty"`
	Format      string  `json:"format"`
	Preset      string  `json:"preset"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	DurationSec float64 `json:"durationSeconds"`
	SourceBytes int64   `json:"sourceBytes"`
	OutputBytes int64   `json:"outputBytes"`
	Reduction   float64 `json:"reduction"`
	EncodedAt   string  `json:"encodedAt"`
}

func writeReport(path string, res Result, thumb domain.ThumbSpec, thumbAt time.Duration) error {
	thumbName := ""
	if thumb.Enabled && res.ThumbPath != "" {
		thumbName = filepath.Base(res.ThumbPath)
	}

	payload := report{
		Source:      filepath.Base(res.Plan.Source.Path),
		Video:       filepath.Base(res.VideoPath),
		Thumbnail:   thumbName,
		ThumbnailAt: thumbAt.Seconds(),
		Format:      res.Plan.Format.ID,
		Preset:      string(res.Plan.Preset.Level),
		Width:       res.Plan.OutputWidth(),
		Height:      res.Plan.OutputHeight(),
		DurationSec: res.Plan.Source.Info.Duration.Seconds(),
		SourceBytes: res.SourceBytes,
		OutputBytes: res.OutputBytes,
		Reduction:   res.Reduction,
		EncodedAt:   time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
