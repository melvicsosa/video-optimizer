// Package fsscan discovers video files on disk.
package fsscan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// Extensions are the container formats the scanner recognises.
var Extensions = []string{".mp4", ".webm", ".mov", ".mkv", ".avi", ".m4v", ".mpg", ".mpeg", ".wmv", ".flv"}

// Scanner lists videos in a single directory, non recursively.
type Scanner struct {
	// Skip receives absolute paths and reports whether they must be ignored,
	// used to keep previously generated outputs out of the list.
	Skip func(path string) bool
}

// New returns a Scanner that ignores everything inside outDir.
func New(outDir string) Scanner {
	abs, err := filepath.Abs(outDir)
	if err != nil {
		abs = outDir
	}
	return Scanner{Skip: func(path string) bool {
		return strings.HasPrefix(path, abs+string(os.PathSeparator))
	}}
}

// Scan returns the videos found in dir, largest first.
func (s Scanner) Scan(dir string) ([]domain.Video, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var videos []domain.Video
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !isVideo(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if s.Skip != nil && s.Skip(abs) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		videos = append(videos, domain.Video{Path: path, SizeBytes: info.Size()})
	}

	sort.Slice(videos, func(i, j int) bool {
		if videos[i].SizeBytes != videos[j].SizeBytes {
			return videos[i].SizeBytes > videos[j].SizeBytes
		}
		return videos[i].Path < videos[j].Path
	})
	return videos, nil
}

func isVideo(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, candidate := range Extensions {
		if ext == candidate {
			return true
		}
	}
	return false
}
