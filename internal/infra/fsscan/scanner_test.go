package fsscan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "output")

	write(t, filepath.Join(dir, "small.mov"), 100)
	write(t, filepath.Join(dir, "big.mp4"), 900)
	write(t, filepath.Join(dir, "medium.webm"), 500)
	write(t, filepath.Join(dir, "notes.txt"), 10)
	write(t, filepath.Join(dir, ".hidden.mp4"), 10)
	write(t, filepath.Join(outDir, "already-encoded.webm"), 400)

	videos, err := New(outDir).Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	var names []string
	for _, v := range videos {
		names = append(names, v.FileName())
	}

	want := []string{"big.mp4", "medium.webm", "small.mov"} // largest first
	if len(names) != len(want) {
		t.Fatalf("found %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("position %d = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestScanMissingDirectory(t *testing.T) {
	if _, err := New("output").Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func TestBaseName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "My Clip.mp4"), 10)

	videos, err := New(filepath.Join(dir, "output")).Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := videos[0].BaseName(); got != "My Clip" {
		t.Errorf("BaseName() = %q, want %q", got, "My Clip")
	}
}
