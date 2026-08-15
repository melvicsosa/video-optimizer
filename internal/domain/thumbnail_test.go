package domain

import (
	"testing"
	"time"
)

func TestParseThumbSpec(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantErr  bool
		enabled  bool
		percent  bool
		resolved time.Duration // against a ten minute video
	}{
		{name: "off disables the frame", raw: "off", enabled: false},
		{name: "none disables the frame", raw: "none", enabled: false},
		{name: "empty falls back to the default", raw: "", enabled: true, percent: true, resolved: time.Minute},
		{name: "percentage of the duration", raw: "25%", enabled: true, percent: true, resolved: 150 * time.Second},
		{name: "plain seconds", raw: "12", enabled: true, resolved: 12 * time.Second},
		{name: "minutes and seconds", raw: "1:30", enabled: true, resolved: 90 * time.Second},
		{name: "past the end is clamped", raw: "99:00", enabled: true, resolved: 10 * time.Minute},
		{name: "negative percentage", raw: "-5%", wantErr: true},
		{name: "percentage above 100", raw: "120%", wantErr: true},
		{name: "garbage", raw: "soon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseThumbSpec(tt.raw)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if spec.Enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", spec.Enabled, tt.enabled)
			}
			if spec.UsePercent != tt.percent {
				t.Errorf("usePercent = %v, want %v", spec.UsePercent, tt.percent)
			}
			if got := spec.For(10 * time.Minute); got != tt.resolved {
				t.Errorf("For(10m) = %s, want %s", got, tt.resolved)
			}
		})
	}
}

func TestThumbSpecForHandlesUnknownDuration(t *testing.T) {
	if got := DefaultThumbSpec().For(0); got != 0 {
		t.Errorf("For(0) = %s, want 0", got)
	}
	if got := NoThumb().For(time.Minute); got != 0 {
		t.Errorf("a disabled spec must resolve to 0, got %s", got)
	}
}

func TestThumbSpecLabel(t *testing.T) {
	tests := []struct {
		name string
		spec ThumbSpec
		want string
	}{
		{name: "disabled", spec: NoThumb(), want: "off"},
		{name: "percentage", spec: DefaultThumbSpec(), want: "10% of each video"},
		{name: "timestamp", spec: ThumbSpec{Enabled: true, At: 90 * time.Second}, want: "1:30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{raw: "0", want: 0},
		{raw: "12", want: 12 * time.Second},
		{raw: "12.5", want: 12500 * time.Millisecond},
		{raw: "1:30", want: 90 * time.Second},
		{raw: "0:05", want: 5 * time.Second},
		{raw: "10:00", want: 10 * time.Minute},
		{raw: "", wantErr: true},
		{raw: "abc", wantErr: true},
		{raw: "-3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseTimestamp(tt.raw)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseTimestamp(%q) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}
