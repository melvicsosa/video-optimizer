package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ThumbSpec describes whether a poster frame is wanted and where it comes from.
//
// A batch mixes videos of different lengths, so the position can be expressed
// as a share of the duration instead of an absolute timestamp.
type ThumbSpec struct {
	Enabled    bool
	UsePercent bool
	Percent    float64 // 0..1, used when UsePercent is true
	At         time.Duration
}

// DefaultThumbSpec places the frame one tenth into the video, past intros and
// black frames.
func DefaultThumbSpec() ThumbSpec {
	return ThumbSpec{Enabled: true, UsePercent: true, Percent: 0.1}
}

// NoThumb disables the poster frame.
func NoThumb() ThumbSpec { return ThumbSpec{} }

// For resolves the spec against a concrete duration.
func (s ThumbSpec) For(duration time.Duration) time.Duration {
	if !s.Enabled || duration <= 0 {
		return 0
	}
	at := s.At
	if s.UsePercent {
		at = time.Duration(float64(duration) * s.Percent)
	}
	if at < 0 {
		return 0
	}
	if at > duration {
		return duration
	}
	return at
}

// Label renders the spec for the CLI and the help output.
func (s ThumbSpec) Label() string {
	switch {
	case !s.Enabled:
		return "off"
	case s.UsePercent:
		return fmt.Sprintf("%.0f%% of each video", s.Percent*100)
	default:
		total := int(s.At.Round(time.Second).Seconds())
		return fmt.Sprintf("%d:%02d", total/60, total%60)
	}
}

// ParseThumbSpec reads the values accepted by the -thumb flag:
//
//	off      no poster frame
//	10%      a share of each video's duration
//	12       twelve seconds into every video
//	1:30     one minute thirty into every video
func ParseThumbSpec(raw string) (ThumbSpec, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "off", "no", "none", "false":
		return NoThumb(), nil
	case "":
		return DefaultThumbSpec(), nil
	}

	if percent, ok := strings.CutSuffix(value, "%"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(percent), 64)
		if err != nil || f < 0 || f > 100 {
			return ThumbSpec{}, fmt.Errorf("invalid thumbnail percentage %q", raw)
		}
		return ThumbSpec{Enabled: true, UsePercent: true, Percent: f / 100}, nil
	}

	at, err := ParseTimestamp(value)
	if err != nil {
		return ThumbSpec{}, fmt.Errorf("invalid thumbnail position %q (use off, 10%%, 12 or 1:30)", raw)
	}
	return ThumbSpec{Enabled: true, At: at}, nil
}

// ParseTimestamp reads "90", "90.5" and "1:30" into a duration.
func ParseTimestamp(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty timestamp")
	}

	if mm, ss, ok := strings.Cut(value, ":"); ok {
		minutes, err := parsePart(mm)
		if err != nil {
			return 0, err
		}
		seconds, err := parsePart(ss)
		if err != nil {
			return 0, err
		}
		return time.Duration((minutes*60 + seconds) * float64(time.Second)), nil
	}

	seconds, err := parsePart(value)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func parsePart(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("invalid timestamp part %q", s)
	}
	return f, nil
}
