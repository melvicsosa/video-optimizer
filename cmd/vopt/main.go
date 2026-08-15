// Command vopt compresses videos with ffmpeg and captures a poster frame.
//
// Run it with no arguments for the interactive flow. Every question it asks can
// be answered up front with a flag; answered questions are skipped, and -y
// skips the interface altogether so the tool fits a script or a CI job.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/melvicsosa/video-optimizer/internal/app"
	"github.com/melvicsosa/video-optimizer/internal/domain"
	"github.com/melvicsosa/video-optimizer/internal/humanize"
	"github.com/melvicsosa/video-optimizer/internal/infra/ffmpeg"
	"github.com/melvicsosa/video-optimizer/internal/infra/fsscan"
	"github.com/melvicsosa/video-optimizer/internal/ui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
// Builds that skip the flag (notably `go install module@version`) fall back
// to the module version Go records in the binary's build info.
var version = "dev"

// resolveVersion returns the ldflags-injected version when present, otherwise
// the module version from build info. "(devel)", what a plain `go build`
// inside the repo without VCS stamping reports, stays "dev".
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok &&
		info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

type options struct {
	dir       string
	out       string
	input     string
	all       bool
	preset    string
	format    string
	thumb     string
	noThumb   bool
	keepNames bool
	noReport  bool
	noAnalyze bool
	yes       bool
	showVer   bool

	// set records which flags the user actually passed, so an untouched flag
	// stays a question instead of becoming a silent default.
	set map[string]bool
}

func main() {
	version = resolveVersion()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	opts := parseFlags()

	if opts.showVer {
		fmt.Println("vopt", version)
		return nil
	}
	if err := requireBinaries("ffmpeg", "ffprobe"); err != nil {
		return err
	}

	outDir := opts.out
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(opts.dir, outDir)
	}

	optimizer := app.Optimizer{
		Encoder:   ffmpeg.NewEncoder(),
		Thumbs:    ffmpeg.NewThumbGrabber(),
		OutDir:    outDir,
		KeepNames: opts.keepNames,
		Report:    !opts.noReport,
	}

	cfg, err := opts.uiConfig(outDir)
	if err != nil {
		return err
	}

	if opts.yes {
		return runHeadless(opts, cfg, optimizer)
	}

	model := ui.New(cfg, fsscan.New(outDir), ffmpeg.NewProber(), ffmpeg.NewSampler(), optimizer)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.dir, "dir", ".", "directory to scan for videos")
	flag.StringVar(&opts.out, "out", "output", "directory for the encoded files")
	flag.StringVar(&opts.input, "input", "", "process this file only")
	flag.BoolVar(&opts.all, "all", false, "process every video in the directory, one after another")
	flag.StringVar(&opts.preset, "preset", "balanced", "compression preset: light, balanced or aggressive")
	flag.StringVar(&opts.format, "format", "webm-vp9", "output format: webm-vp9, webm-av1, mp4-h265 or mp4-h264")
	flag.StringVar(&opts.thumb, "thumb", "10%", "thumbnail position: off, 10%, 12 or 1:30")
	flag.BoolVar(&opts.noThumb, "no-thumb", false, "do not capture a thumbnail")
	flag.BoolVar(&opts.keepNames, "keep-names", false, "keep the original file name instead of a web friendly slug")
	flag.BoolVar(&opts.noReport, "no-report", false, "do not write the JSON report next to the output")
	flag.BoolVar(&opts.noAnalyze, "no-analyze", false, "skip the sample based size prediction")
	flag.BoolVar(&opts.yes, "y", false, "run without asking anything, using the flags and their defaults")
	flag.BoolVar(&opts.showVer, "version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()

	opts.set = map[string]bool{}
	flag.Visit(func(f *flag.Flag) { opts.set[f.Name] = true })
	return opts
}

// uiConfig turns the flags into the flow's preselections. A nil field means the
// question is still open and will be asked.
func (o options) uiConfig(outDir string) (ui.Config, error) {
	cfg := ui.Config{
		Dir:     o.dir,
		OutDir:  outDir,
		Version: version,
		Analyze: !o.noAnalyze,
	}

	switch {
	case o.input != "":
		cfg.Target = ui.TargetFile
		cfg.InputPath = o.input
	case o.all, o.yes:
		cfg.Target = ui.TargetAll
	default:
		cfg.Target = ui.TargetAsk
	}

	if o.set["preset"] || o.yes {
		preset, ok := domain.PresetByLevel(domain.Level(o.preset))
		if !ok {
			return cfg, fmt.Errorf("unknown preset %q (light, balanced, aggressive)", o.preset)
		}
		cfg.Preset = &preset
	}
	if o.set["format"] || o.yes {
		format, ok := domain.FormatByID(o.format)
		if !ok {
			return cfg, fmt.Errorf("unknown format %q (webm-vp9, webm-av1, mp4-h265, mp4-h264)", o.format)
		}
		cfg.Format = &format
	}

	switch {
	case o.noThumb:
		spec := domain.NoThumb()
		cfg.Thumb = &spec
	case o.set["thumb"], o.yes:
		spec, err := domain.ParseThumbSpec(o.thumb)
		if err != nil {
			return cfg, err
		}
		cfg.Thumb = &spec
	}

	return cfg, nil
}

// runHeadless does the same work as the interactive flow, printing plain lines
// so the tool composes well with scripts and CI.
func runHeadless(opts options, cfg ui.Config, optimizer app.Optimizer) error {
	ctx := context.Background()

	queue, err := resolveQueue(ctx, opts, cfg)
	if err != nil {
		return err
	}

	preset, format, thumb := *cfg.Preset, *cfg.Format, *cfg.Thumb
	fmt.Printf("%d video(s) · %s · %s · thumbnail %s\n\n",
		len(queue), format.Name, preset.Name, thumb.Label())

	var totalSource, totalOutput int64
	for i, video := range queue {
		var complexity domain.Complexity
		if cfg.Analyze {
			measured, err := ffmpeg.NewSampler().Sample(ctx, domain.SamplePlan(video))
			if err != nil {
				fmt.Fprintln(os.Stderr, "warning: could not measure", video.FileName()+":", err)
			} else {
				complexity = measured
			}
		}

		plan := domain.BuildPlanWith(video, preset, format, complexity)
		fmt.Printf("[%d/%d] %s  %s · %s · %s\n", i+1, len(queue), video.FileName(),
			humanize.Bytes(video.SizeBytes), humanize.Duration(video.Info.Duration), video.Info.ResolutionLabel())
		fmt.Printf("        target %s%s (−%s) · CRF %d · ceiling %s\n",
			estimatePrefix(plan), humanize.Bytes(plan.EstimatedBytes),
			humanize.Percent(plan.EstimatedReduction), plan.CRF, humanize.Bitrate(plan.VideoBitrate))

		// Live progress only makes sense on a terminal; in a log it would just
		// be thousands of half written lines.
		var onProgress func(domain.Progress)
		if isTerminal(os.Stdout) {
			lastPrint := time.Time{}
			onProgress = func(p domain.Progress) {
				if video.Info.Duration <= 0 || time.Since(lastPrint) < time.Second {
					return
				}
				lastPrint = time.Now()
				fmt.Printf("\r        %3.0f%%  %s / %s  %.1fx      ",
					p.Elapsed.Seconds()/video.Info.Duration.Seconds()*100,
					humanize.Duration(p.Elapsed), humanize.Duration(video.Info.Duration), p.Speed)
			}
		}

		res, err := optimizer.Run(ctx, plan, thumb, onProgress)
		if onProgress != nil {
			fmt.Print("\r\033[K")
		}
		if err != nil {
			return err
		}

		totalSource += res.SourceBytes
		totalOutput += res.OutputBytes

		fmt.Printf("        %s  %s → %s  (−%s in %s)\n",
			res.VideoPath, humanize.Bytes(res.SourceBytes), humanize.Bytes(res.OutputBytes),
			humanize.Percent(res.Reduction), humanize.Duration(res.Took))
		if res.ThumbPath != "" {
			fmt.Printf("        %s\n", res.ThumbPath)
		}
		fmt.Println()
	}

	if len(queue) > 1 && totalSource > 0 {
		fmt.Printf("total: %s → %s  (−%s)\n",
			humanize.Bytes(totalSource), humanize.Bytes(totalOutput),
			humanize.Percent(1-float64(totalOutput)/float64(totalSource)))
	}
	return nil
}

// resolveQueue lists and probes the videos a headless run should process.
func resolveQueue(ctx context.Context, opts options, cfg ui.Config) ([]domain.Video, error) {
	prober := ffmpeg.NewProber()

	probe := func(path string) (domain.Video, error) {
		stat, err := os.Stat(path)
		if err != nil {
			return domain.Video{}, err
		}
		info, err := prober.Probe(ctx, path)
		if err != nil {
			return domain.Video{}, err
		}
		if info.SizeBytes == 0 {
			info.SizeBytes = stat.Size()
		}
		return domain.Video{Path: path, SizeBytes: stat.Size(), Info: info, Probed: true}, nil
	}

	if cfg.Target == ui.TargetFile {
		video, err := probe(opts.input)
		if err != nil {
			return nil, err
		}
		return []domain.Video{video}, nil
	}

	found, err := fsscan.New(cfg.OutDir).Scan(cfg.Dir)
	if err != nil {
		return nil, err
	}

	var queue []domain.Video
	for _, candidate := range found {
		video, err := probe(candidate.Path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "skipping", candidate.FileName()+":", err)
			continue
		}
		queue = append(queue, video)
	}
	if len(queue) == 0 {
		return nil, errors.New("no readable videos found in " + cfg.Dir)
	}
	return queue, nil
}

// isTerminal reports whether f is attached to a terminal rather than a pipe.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// estimatePrefix distinguishes a measured prediction (≈) from a plain upper
// bound (≤), mirroring the interactive flow.
func estimatePrefix(plan domain.EncodePlan) string {
	if plan.Measured {
		return "≈ "
	}
	return "≤ "
}

func requireBinaries(names ...string) error {
	var missing []string
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s not found in PATH — install it with: brew install ffmpeg",
			strings.Join(missing, " and "))
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `vopt %s — compress videos and capture their poster frame.

Usage:
  vopt                                    interactive flow in the current directory
  vopt -all                               interactive, but process every video
  vopt -input clip.mp4 -preset light      interactive, with two answers already given
  vopt -y -all -preset aggressive         no questions at all
  vopt -y -input clip.mp4 -no-thumb       one file, video only

Any flag you pass is a question the interactive flow will not ask.

Flags:
`, version)
	flag.PrintDefaults()
}
