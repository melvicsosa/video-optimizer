// Package humanize renders raw numbers as the short, readable strings the CLI
// and the JSON report expose to the user.
package humanize

import (
	"fmt"
	"time"
)

// Bytes renders a byte count using decimal units, e.g. "347.2 MB".
func Bytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGT"[exp])
}

// Bitrate renders bits per second, e.g. "9.5 Mbps".
func Bitrate(bps int64) string {
	switch {
	case bps <= 0:
		return "n/a"
	case bps >= 1_000_000:
		return fmt.Sprintf("%.1f Mbps", float64(bps)/1_000_000)
	case bps >= 1_000:
		return fmt.Sprintf("%.0f kbps", float64(bps)/1_000)
	default:
		return fmt.Sprintf("%d bps", bps)
	}
}

// Duration renders a duration as mm:ss, or h:mm:ss past the hour mark.
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second).Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Percent renders a 0..1 ratio as a signed percentage, e.g. "-65%".
func Percent(ratio float64) string {
	return fmt.Sprintf("%.0f%%", ratio*100)
}
