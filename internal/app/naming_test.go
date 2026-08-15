package app

import "testing"

func TestSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "spaces become hyphens", input: "video promo 01", want: "video-promo-01"},
		{name: "accents are folded", input: "vinculación con el presupuesto", want: "vinculacion-con-el-presupuesto"},
		{name: "eñe is folded", input: "Diseño Año 2026", want: "diseno-ano-2026"},
		{name: "punctuation collapses", input: "clip__final!! (v2)", want: "clip-final-v2"},
		{name: "edges are trimmed", input: "  -- hola --  ", want: "hola"},
		{name: "unmappable names fall back", input: "日本語", want: "video"},
		{name: "empty falls back", input: "", want: "video"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slug(tt.input); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
