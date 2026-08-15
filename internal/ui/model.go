// Package ui implements the interactive terminal flow: pick what to compress,
// review it, choose how hard to compress, pick a format, decide on a poster
// frame and encode.
//
// Every question can be answered up front with a flag. Whatever the flags
// already answer is skipped here, so the same binary serves an exploratory
// session and a scripted one.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/melvicsosa/video-optimizer/internal/app"
	"github.com/melvicsosa/video-optimizer/internal/domain"
)

type step int

const (
	stepScanning step = iota
	stepPicking
	stepSampling
	stepDetails
	stepPreset
	stepFormat
	stepThumbAsk
	stepThumb
	stepEncoding
	stepDone
	stepFatal
)

// Target says what the run should process.
type Target int

const (
	TargetAsk  Target = iota // let the user choose in the picker
	TargetAll                // every video in the directory
	TargetFile               // one specific file
)

// Config carries the runtime options and the answers already given by flags.
// A nil preselection means the flow asks for it.
type Config struct {
	Dir     string
	OutDir  string
	Version string

	Target    Target
	InputPath string
	Preset    *domain.Preset
	Format    *domain.Format
	Thumb     *domain.ThumbSpec
	Analyze   bool
}

// Model is the root bubbletea model driving the whole flow.
type Model struct {
	cfg     Config
	scanner domain.Scanner
	prober  domain.Prober
	sampler domain.Sampler
	opt     app.Optimizer

	step   step
	width  int
	height int

	spinner spinner.Model
	bar     progress.Model

	videos []domain.Video
	cursor int

	// queue holds the videos to process: one file, or every video in the
	// directory when the batch entry is chosen.
	batch        bool
	queue        []domain.Video
	queueIndex   int
	sampleIndex  int
	complexities map[string]domain.Complexity
	sampleErr    error

	presetCursor int
	formatCursor int
	plan         domain.EncodePlan

	// thumbAt places the poster frame in single mode; thumbPercent does the
	// same across a batch, where every video has a different duration.
	thumbEnabled bool
	thumbAt      time.Duration
	thumbPercent float64
	thumbBuf     string
	thumbAskYes  bool
	previewPath  string
	previewErr   error

	progress   domain.Progress
	cancel     context.CancelFunc
	progressCh chan domain.Progress

	results []app.Result
	err     error
}

// New builds the root model with its adapters already wired.
func New(cfg Config, scanner domain.Scanner, prober domain.Prober, sampler domain.Sampler, opt app.Optimizer) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleAccent

	// The bar sweeps the brand gradient, green into blue.
	bar := progress.New(
		progress.WithScaledGradient("#4ADE80", "#3B82F6"),
		progress.WithoutPercentage(),
	)

	return Model{
		cfg:          cfg,
		scanner:      scanner,
		prober:       prober,
		sampler:      sampler,
		opt:          opt,
		step:         stepScanning,
		spinner:      sp,
		bar:          bar,
		width:        90,
		presetCursor: 1, // balanced is the sensible default
		thumbEnabled: true,
		thumbAskYes:  true,
		thumbPercent: 0.1,
		complexities: map[string]domain.Complexity{},
	}
}

// Init starts the scan as soon as the program runs.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.scanCmd())
}

type scannedMsg struct{ videos []domain.Video }
type sampledMsg struct {
	path       string
	complexity domain.Complexity
	err        error
}
type fatalMsg struct{ err error }
type progressMsg domain.Progress
type encodeDoneMsg struct{ res app.Result }
type encodeFailedMsg struct{ err error }
type previewReadyMsg struct{ path string }
type previewClosedMsg struct{ err error }

// scanCmd lists the directory and probes every candidate concurrently, so the
// picker can show duration and resolution right away.
func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := m.scanner.Scan(m.cfg.Dir)
		if err != nil {
			return fatalMsg{err: err}
		}
		if len(videos) == 0 {
			return fatalMsg{err: fmt.Errorf("no video files found in %s", m.cfg.Dir)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var wg sync.WaitGroup
		for i := range videos {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				info, err := m.prober.Probe(ctx, videos[i].Path)
				if err != nil {
					videos[i].ProbeErr = err
					return
				}
				if info.SizeBytes == 0 {
					info.SizeBytes = videos[i].SizeBytes
				}
				videos[i].Info = info
				videos[i].Probed = true
			}(i)
		}
		wg.Wait()

		return scannedMsg{videos: videos}
	}
}

// Update routes messages to the active step.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.bar.Width = clamp(msg.Width-20, 20, 60)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scannedMsg:
		m.videos = msg.videos
		return m.afterScan()

	case sampledMsg:
		// A failed measurement is not fatal: we fall back to the upper bound.
		if msg.err != nil {
			m.sampleErr = msg.err
		} else {
			m.complexities[msg.path] = msg.complexity
		}
		m.sampleIndex++
		if m.sampleIndex < len(m.queue) {
			return m, m.sampleCmd(m.queue[m.sampleIndex])
		}
		m.step = stepDetails
		return m, nil

	case fatalMsg:
		m.err = msg.err
		m.step = stepFatal
		return m, nil

	case progressMsg:
		m.progress = domain.Progress(msg)
		return m, m.waitProgress()

	case encodeDoneMsg:
		return m.afterEncode(msg.res)

	case encodeFailedMsg:
		m.err = msg.err
		m.step = stepFatal
		m.cancel = nil
		return m, nil

	case previewReadyMsg:
		m.previewPath = msg.path
		m.previewErr = nil
		return m, tea.ExecProcess(previewCommand(msg.path), func(err error) tea.Msg {
			return previewClosedMsg{err: err}
		})

	case previewClosedMsg:
		m.previewErr = msg.err
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// afterScan honours the target chosen by flags, skipping the picker when the
// run already knows what to work on.
func (m Model) afterScan() (tea.Model, tea.Cmd) {
	switch m.cfg.Target {
	case TargetAll:
		return m.startQueue(readable(m.videos), true)

	case TargetFile:
		wanted, err := filepath.Abs(m.cfg.InputPath)
		if err != nil {
			return m.fatal(err)
		}
		for _, v := range m.videos {
			if abs, err := filepath.Abs(v.Path); err == nil && abs == wanted {
				if !v.Probed {
					return m.fatal(fmt.Errorf("%s could not be read by ffprobe", v.FileName()))
				}
				return m.startQueue([]domain.Video{v}, false)
			}
		}
		return m.fatal(fmt.Errorf("%s is not in %s", filepath.Base(m.cfg.InputPath), m.cfg.Dir))

	default:
		m.step = stepPicking
		return m, nil
	}
}

func (m Model) fatal(err error) (tea.Model, tea.Cmd) {
	m.err = err
	m.step = stepFatal
	return m, nil
}

// startQueue locks in what will be processed and measures it.
func (m Model) startQueue(videos []domain.Video, batch bool) (tea.Model, tea.Cmd) {
	if len(videos) == 0 {
		return m.fatal(errors.New("no readable videos to process"))
	}

	m.queue = videos
	m.batch = batch
	m.queueIndex = 0
	m.sampleIndex = 0
	m.sampleErr = nil
	m.complexities = map[string]domain.Complexity{}
	m.thumbAt = defaultThumbAt(videos[0].Info.Duration)
	m.thumbBuf = m.defaultThumbBuffer()

	if !m.cfg.Analyze {
		m.step = stepDetails
		return m, nil
	}
	m.step = stepSampling
	return m, tea.Batch(m.sampleCmd(videos[0]), m.spinner.Tick)
}

// sampleCmd measures how demanding a source is, so the presets can show a
// realistic size instead of a worst case.
func (m Model) sampleCmd(v domain.Video) tea.Cmd {
	sampler := m.sampler
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		complexity, err := sampler.Sample(ctx, domain.SamplePlan(v))
		return sampledMsg{path: v.Path, complexity: complexity, err: err}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "q":
		// While typing a timestamp "q" is not a quit shortcut.
		if m.step != stepThumb {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	}

	switch m.step {
	case stepPicking:
		return m.updatePicking(msg)
	case stepDetails:
		return m.updateDetails(msg)
	case stepPreset:
		return m.updatePreset(msg)
	case stepFormat:
		return m.updateFormat(msg)
	case stepThumbAsk:
		return m.updateThumbAsk(msg)
	case stepThumb:
		return m.updateThumb(msg)
	case stepEncoding:
		if msg.String() == "esc" && m.cancel != nil {
			m.cancel()
		}
		return m, nil
	case stepDone:
		return m.updateDone(msg)
	case stepFatal:
		if msg.String() == "enter" || msg.String() == "esc" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// pickerRows is the number of selectable entries: every video plus the batch
// entry when there is more than one.
func (m Model) pickerRows() int {
	if len(m.videos) > 1 {
		return len(m.videos) + 1
	}
	return len(m.videos)
}

// batchRow reports whether the cursor sits on the "all videos" entry.
func (m Model) batchRow() bool { return len(m.videos) > 1 && m.cursor == 0 }

// videoAt maps a cursor position to a video.
func (m Model) videoAt(cursor int) domain.Video {
	if len(m.videos) > 1 {
		cursor--
	}
	if cursor < 0 || cursor >= len(m.videos) {
		return domain.Video{}
	}
	return m.videos[cursor]
}

func (m Model) updatePicking(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.cursor = wrap(m.cursor-1, m.pickerRows())
	case "down", "j":
		m.cursor = wrap(m.cursor+1, m.pickerRows())
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = m.pickerRows() - 1
	case "a":
		if len(m.videos) > 1 {
			return m.startQueue(readable(m.videos), true)
		}
	case "enter":
		if m.batchRow() {
			return m.startQueue(readable(m.videos), true)
		}
		v := m.videoAt(m.cursor)
		if !v.Probed {
			return m, nil
		}
		return m.startQueue([]domain.Video{v}, false)
	}
	return m, nil
}

func (m Model) updateDetails(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.back(stepDetails)
	case "enter":
		return m.advance(stepDetails)
	}
	return m, nil
}

func (m Model) updatePreset(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.presetCursor = wrap(m.presetCursor-1, len(domain.Presets))
	case "down", "j":
		m.presetCursor = wrap(m.presetCursor+1, len(domain.Presets))
	case "esc":
		return m.back(stepPreset)
	case "enter":
		return m.advance(stepPreset)
	}
	return m, nil
}

func (m Model) updateFormat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.formatCursor = wrap(m.formatCursor-1, len(domain.Formats))
	case "down", "j":
		m.formatCursor = wrap(m.formatCursor+1, len(domain.Formats))
	case "esc":
		return m.back(stepFormat)
	case "enter":
		return m.advance(stepFormat)
	}
	return m, nil
}

func (m Model) updateThumbAsk(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "down", "j", "left", "right", "tab":
		m.thumbAskYes = !m.thumbAskYes
	case "y":
		m.thumbAskYes = true
		m.thumbEnabled = true
		return m.advance(stepThumbAsk)
	case "n":
		m.thumbAskYes = false
		m.thumbEnabled = false
		return m.advance(stepThumbAsk)
	case "esc":
		return m.back(stepThumbAsk)
	case "enter":
		m.thumbEnabled = m.thumbAskYes
		return m.advance(stepThumbAsk)
	}
	return m, nil
}

func (m Model) updateThumb(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key := msg.String(); key {
	case "esc":
		return m.back(stepThumb)
	case "left":
		m.nudgeThumb(-1)
	case "right":
		m.nudgeThumb(1)
	case "down", "j":
		m.nudgeThumb(-10)
	case "up", "k":
		m.nudgeThumb(10)
	case "backspace":
		if len(m.thumbBuf) > 0 {
			m.thumbBuf = m.thumbBuf[:len(m.thumbBuf)-1]
			m.syncThumbFromBuffer()
		}
	case "p":
		return m, m.previewCmd()
	case "enter":
		m.syncThumbFromBuffer()
		return m.advance(stepThumb)
	default:
		if len(key) == 1 && (key[0] == ':' || key[0] == '.' || (key[0] >= '0' && key[0] <= '9')) {
			m.thumbBuf += key
			m.syncThumbFromBuffer()
		}
	}
	return m, nil
}

// nudgeThumb moves the poster frame: whole seconds for a single video, whole
// percentage points for a batch.
func (m *Model) nudgeThumb(delta int) {
	if m.batch {
		m.thumbPercent = clampFloat(m.thumbPercent+float64(delta)/100, 0, 1)
		m.thumbBuf = strconv.Itoa(int(m.thumbPercent*100 + 0.5))
		return
	}
	m.thumbAt = clampDuration(m.thumbAt+time.Duration(delta)*time.Second, m.reference().Info.Duration)
	m.thumbBuf = formatTimestamp(m.thumbAt)
}

func (m *Model) syncThumbFromBuffer() {
	if m.batch {
		if percent, err := strconv.ParseFloat(strings.TrimSuffix(m.thumbBuf, "%"), 64); err == nil {
			m.thumbPercent = clampFloat(percent/100, 0, 1)
		}
		return
	}
	if d, ok := parseTimestamp(m.thumbBuf); ok {
		m.thumbAt = clampDuration(d, m.reference().Info.Duration)
	}
}

func (m Model) defaultThumbBuffer() string {
	if m.batch {
		return strconv.Itoa(int(m.thumbPercent*100 + 0.5))
	}
	return formatTimestamp(m.thumbAt)
}

func (m Model) updateDone(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		return m, tea.Quit
	case "o":
		return m, tea.ExecProcess(openDirCommand(m.opt.OutDir), func(error) tea.Msg { return nil })
	case "n":
		if m.cfg.Target != TargetAsk {
			return m, tea.Quit
		}
		m.step = stepPicking
		m.results = nil
		m.queue = nil
		m.progress = domain.Progress{}
		return m, m.scanCmd()
	}
	return m, nil
}

// needs reports whether a step still has a question to ask.
func (m Model) needs(s step) bool {
	switch s {
	case stepDetails:
		return true
	case stepPreset:
		return m.cfg.Preset == nil
	case stepFormat:
		return m.cfg.Format == nil
	case stepThumbAsk:
		return m.cfg.Thumb == nil
	case stepThumb:
		return m.cfg.Thumb == nil && m.thumbEnabled
	default:
		return false
	}
}

// advance moves to the next step that still needs an answer, starting the
// encode once everything is decided.
func (m Model) advance(from step) (tea.Model, tea.Cmd) {
	for s := from + 1; s < stepEncoding; s++ {
		if m.needs(s) {
			m.step = s
			return m, nil
		}
	}
	return m.startEncode()
}

// back walks the same path in reverse.
func (m Model) back(from step) (tea.Model, tea.Cmd) {
	for s := from - 1; s > stepSampling; s-- {
		if m.needs(s) {
			m.step = s
			return m, nil
		}
	}
	if m.cfg.Target == TargetAsk {
		m.step = stepPicking
		return m, nil
	}
	m.step = from
	return m, nil
}

// thumbSpec resolves the poster frame decision from the flags or the answers
// given during the flow.
func (m Model) thumbSpec() domain.ThumbSpec {
	if m.cfg.Thumb != nil {
		return *m.cfg.Thumb
	}
	if !m.thumbEnabled {
		return domain.NoThumb()
	}
	if m.batch {
		return domain.ThumbSpec{Enabled: true, UsePercent: true, Percent: m.thumbPercent}
	}
	return domain.ThumbSpec{Enabled: true, At: m.thumbAt}
}

func (m Model) preset() domain.Preset {
	if m.cfg.Preset != nil {
		return *m.cfg.Preset
	}
	return domain.Presets[m.presetCursor]
}

func (m Model) format() domain.Format {
	if m.cfg.Format != nil {
		return *m.cfg.Format
	}
	return domain.Formats[m.formatCursor]
}

// currentPlan resolves the plan for the video being encoded.
func (m Model) currentPlan() domain.EncodePlan {
	return m.planFor(m.current(), m.preset(), m.format())
}

// planFor resolves a plan for an arbitrary combination, used by the menus to
// preview every option.
func (m Model) planFor(v domain.Video, preset domain.Preset, format domain.Format) domain.EncodePlan {
	return domain.BuildPlanWith(v, preset, format, m.complexityOf(v))
}

// current is the video being configured or encoded.
func (m Model) current() domain.Video {
	if len(m.queue) == 0 {
		return domain.Video{}
	}
	if m.queueIndex >= len(m.queue) {
		return m.queue[len(m.queue)-1]
	}
	return m.queue[m.queueIndex]
}

// reference is the video the configuration screens describe. In a batch that is
// the first one, used to lay out examples.
func (m Model) reference() domain.Video {
	if len(m.queue) == 0 {
		return domain.Video{}
	}
	return m.queue[0]
}

func (m Model) complexityOf(v domain.Video) domain.Complexity {
	return m.complexities[v.Path]
}

// measured reports whether every queued video could be measured.
func (m Model) measured() bool {
	if len(m.queue) == 0 {
		return false
	}
	for _, v := range m.queue {
		if !m.complexityOf(v).Valid() {
			return false
		}
	}
	return true
}

// queueTotals adds up the source sizes and the estimates for the whole queue.
func (m Model) queueTotals(preset domain.Preset, format domain.Format) (source, estimated int64, measured bool) {
	measured = len(m.queue) > 0
	for _, v := range m.queue {
		plan := m.planFor(v, preset, format)
		source += v.SizeBytes
		estimated += plan.EstimatedBytes
		if !plan.Measured {
			measured = false
		}
	}
	return source, estimated, measured
}

// previewCmd captures the frame at the current position into a temp file and
// hands it to the system viewer.
func (m Model) previewCmd() tea.Cmd {
	video := m.reference()
	at := m.thumbSpec().For(video.Info.Duration)
	height := m.planFor(video, m.preset(), m.format()).OutputHeight()
	grabber := m.opt.Thumbs

	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "vopt-preview")
		if err != nil {
			return previewClosedMsg{err: err}
		}
		dst := filepath.Join(dir, "frame.jpg")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := grabber.Grab(ctx, video.Path, dst, at, height); err != nil {
			return previewClosedMsg{err: err}
		}
		return previewReadyMsg{path: dst}
	}
}

// startEncode kicks off the pipeline for the current queue entry.
func (m Model) startEncode() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan domain.Progress, 64)

	m.cancel = cancel
	m.progressCh = ch
	m.step = stepEncoding
	m.progress = domain.Progress{}
	m.plan = m.currentPlan()

	plan, thumb, opt := m.plan, m.thumbSpec(), m.opt

	run := func() tea.Msg {
		defer cancel()
		res, err := opt.Run(ctx, plan, thumb, func(p domain.Progress) {
			select {
			case ch <- p:
			default: // never block ffmpeg on a slow UI
			}
		})
		close(ch)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return encodeFailedMsg{err: errors.New("encode cancelled")}
			}
			return encodeFailedMsg{err: err}
		}
		return encodeDoneMsg{res: res}
	}

	return m, tea.Batch(run, m.waitProgress(), m.spinner.Tick)
}

// afterEncode records the result and moves on to the next video in the queue.
func (m Model) afterEncode(res app.Result) (tea.Model, tea.Cmd) {
	m.results = append(m.results, res)
	m.cancel = nil
	m.queueIndex++

	if m.queueIndex < len(m.queue) {
		return m.startEncode()
	}
	m.step = stepDone
	return m, nil
}

func (m Model) waitProgress() tea.Cmd {
	ch := m.progressCh
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return progressMsg(p)
	}
}

// readable keeps only the videos ffprobe could open.
func readable(videos []domain.Video) []domain.Video {
	var out []domain.Video
	for _, v := range videos {
		if v.Probed {
			out = append(out, v)
		}
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func wrap(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}

func clampDuration(d, max time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if max > 0 && d > max {
		return max
	}
	return d
}

// defaultThumbAt places the initial poster frame slightly into the video, past
// intros and black frames.
func defaultThumbAt(total time.Duration) time.Duration {
	if total <= 0 {
		return time.Second
	}
	at := time.Duration(float64(total) * 0.1)
	if at < time.Second {
		at = time.Second
	}
	return clampDuration(at, total)
}

func formatTimestamp(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func parseTimestamp(s string) (time.Duration, bool) {
	d, err := domain.ParseTimestamp(s)
	if err != nil {
		return 0, false
	}
	return d, true
}
