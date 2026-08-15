package domain

import (
	"context"
	"time"
)

// Scanner finds candidate video files inside a directory.
type Scanner interface {
	Scan(dir string) ([]Video, error)
}

// Prober reads the technical metadata of a media file.
type Prober interface {
	Probe(ctx context.Context, path string) (MediaInfo, error)
}

// Progress is a snapshot of an ongoing encode.
type Progress struct {
	Elapsed time.Duration // position inside the source timeline
	FPS     float64
	Speed   float64 // encoding speed relative to realtime, e.g. 1.4 means 1.4x
	Bitrate int64
}

// Encoder turns a plan into an output file, reporting progress as it goes.
type Encoder interface {
	Encode(ctx context.Context, plan EncodePlan, dst string, onProgress func(Progress)) error
}

// Sampler measures how demanding a source is by encoding a few short excerpts
// with the settings described by plan.
type Sampler interface {
	Sample(ctx context.Context, plan EncodePlan) (Complexity, error)
}

// ThumbGrabber extracts a single frame as an image file.
type ThumbGrabber interface {
	Grab(ctx context.Context, src, dst string, at time.Duration, height int) error
}
