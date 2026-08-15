package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/melvicsosa/video-optimizer/internal/app"
	"github.com/melvicsosa/video-optimizer/internal/domain"
)

// --- fakes -------------------------------------------------------------------

type fakeScanner struct {
	videos []domain.Video
	err    error
}

func (f fakeScanner) Scan(string) ([]domain.Video, error) { return f.videos, f.err }

type fakeProber struct{ info domain.MediaInfo }

func (f fakeProber) Probe(context.Context, string) (domain.MediaInfo, error) { return f.info, nil }

type fakeSampler struct {
	complexity domain.Complexity
	err        error
}

func (f fakeSampler) Sample(context.Context, domain.EncodePlan) (domain.Complexity, error) {
	return f.complexity, f.err
}

type fakeEncoder struct{ err error }

func (f fakeEncoder) Encode(_ context.Context, _ domain.EncodePlan, dst string, onProgress func(domain.Progress)) error {
	if f.err != nil {
		return f.err
	}
	if onProgress != nil {
		onProgress(domain.Progress{Elapsed: 30 * time.Second, Speed: 2})
	}
	return os.WriteFile(dst, []byte("encoded"), 0o644)
}

type fakeThumbs struct{ grabbed int }

func (f *fakeThumbs) Grab(_ context.Context, _, dst string, _ time.Duration, _ int) error {
	f.grabbed++
	return os.WriteFile(dst, []byte("jpeg"), 0o644)
}

// --- helpers -----------------------------------------------------------------

func testVideo(name string, duration time.Duration) domain.Video {
	info := domain.MediaInfo{
		Duration:     duration,
		Width:        1920,
		Height:       1080,
		FPS:          30,
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		VideoBitrate: 9_000_000,
		AudioBitrate: 128_000,
		SizeBytes:    340_000_000,
		HasAudio:     true,
	}
	return domain.Video{Path: name, SizeBytes: info.SizeBytes, Info: info, Probed: true}
}

func testComplexity() domain.Complexity {
	return domain.Complexity{
		RefBitrate: 900_000, RefCRF: 33, RefPixels: 1920 * 1080, RefEfficiency: 1, Samples: 3,
	}
}

// newTestModel returns a model sitting on the picker with the given videos.
func newTestModel(t *testing.T, videos ...domain.Video) Model {
	t.Helper()

	if len(videos) == 0 {
		videos = []domain.Video{testVideo("clip.mp4", 5*time.Minute)}
	}
	m := New(
		Config{Dir: ".", OutDir: t.TempDir(), Analyze: true},
		fakeScanner{videos: videos},
		fakeProber{info: videos[0].Info},
		fakeSampler{complexity: testComplexity()},
		app.Optimizer{Encoder: fakeEncoder{}, Thumbs: &fakeThumbs{}, OutDir: t.TempDir(), Report: true},
	)
	m.videos = videos
	m.step = stepPicking
	return m
}

// queued returns a model with the queue already resolved and measured, as if
// the user had made it past the analysis step.
func queued(t *testing.T, batch bool, videos ...domain.Video) Model {
	t.Helper()

	m := newTestModel(t, videos...)
	m.queue = m.videos
	m.batch = batch
	m.thumbAt = defaultThumbAt(m.videos[0].Info.Duration)
	m.thumbBuf = m.defaultThumbBuffer()
	for _, v := range m.videos {
		m.complexities[v.Path] = testComplexity()
	}
	m.step = stepDetails
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// --- discovery ---------------------------------------------------------------

func TestScanFailureIsFatal(t *testing.T) {
	m := newTestModel(t)
	m.scanner = fakeScanner{err: errors.New("permission denied")}

	msg := m.scanCmd()()
	if _, ok := msg.(fatalMsg); !ok {
		t.Fatalf("got %T, want fatalMsg", msg)
	}
	if got := send(t, m, msg).step; got != stepFatal {
		t.Errorf("step = %v, want stepFatal", got)
	}
}

func TestEmptyDirectoryIsFatal(t *testing.T) {
	m := newTestModel(t)
	m.scanner = fakeScanner{}

	if msg := m.scanCmd()(); msg == nil {
		t.Fatal("expected a message")
	} else if _, ok := msg.(fatalMsg); !ok {
		t.Fatalf("got %T, want fatalMsg when no videos are found", msg)
	}
}

// --- picker ------------------------------------------------------------------

func TestPickerOffersBatchOnlyWithSeveralVideos(t *testing.T) {
	single := newTestModel(t, testVideo("a.mp4", time.Minute))
	if single.pickerRows() != 1 {
		t.Errorf("rows = %d, want 1 for a single video", single.pickerRows())
	}

	many := newTestModel(t, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute))
	if many.pickerRows() != 3 {
		t.Errorf("rows = %d, want 3 (two videos plus the batch entry)", many.pickerRows())
	}
	if !many.batchRow() {
		t.Error("cursor 0 should be the batch entry")
	}
}

func TestPickerNavigationWraps(t *testing.T) {
	videos := []domain.Video{
		testVideo("a.mp4", time.Minute),
		testVideo("b.mp4", time.Minute),
	}

	tests := []struct {
		name  string
		start int
		key   tea.KeyMsg
		want  int
	}{
		{name: "down moves forward", start: 0, key: key("down"), want: 1},
		{name: "j moves forward", start: 1, key: key("j"), want: 2},
		{name: "down wraps to the top", start: 2, key: key("down"), want: 0},
		{name: "up wraps to the bottom", start: 0, key: key("up"), want: 2},
		{name: "k moves back", start: 2, key: key("k"), want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, videos...)
			m.cursor = tt.start

			if got := send(t, m, tt.key).cursor; got != tt.want {
				t.Errorf("cursor = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSelectingOneVideoQueuesOnlyThatVideo(t *testing.T) {
	m := newTestModel(t, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute))
	m.cursor = 2 // the batch entry sits at 0, so this is "b.mp4"

	m = send(t, m, key("enter"))

	if m.batch {
		t.Error("selecting one video must not enable batch mode")
	}
	if len(m.queue) != 1 || m.queue[0].FileName() != "b.mp4" {
		t.Errorf("queue = %v, want just b.mp4", m.queue)
	}
}

func TestBatchEntryQueuesEveryReadableVideo(t *testing.T) {
	broken := domain.Video{Path: "broken.mp4"}
	m := newTestModel(t, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute), broken)
	m.cursor = 0

	m = send(t, m, key("enter"))

	if !m.batch {
		t.Error("the batch entry must enable batch mode")
	}
	if len(m.queue) != 2 {
		t.Errorf("queue has %d videos, want 2: unreadable files are dropped", len(m.queue))
	}
}

func TestShortcutQueuesEveryVideo(t *testing.T) {
	m := newTestModel(t, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute))
	m.cursor = 1

	if m = send(t, m, key("a")); !m.batch {
		t.Error("a should queue every video")
	}
}

func TestUnreadableVideoCannotBeSelected(t *testing.T) {
	m := newTestModel(t, domain.Video{Path: "broken.mp4"})

	if got := send(t, m, key("enter")).step; got != stepPicking {
		t.Errorf("step = %v, want to stay on stepPicking", got)
	}
}

// --- navigation --------------------------------------------------------------

func TestStepTransitions(t *testing.T) {
	tests := []struct {
		name string
		from step
		key  tea.KeyMsg
		want step
	}{
		{name: "analysis to preset", from: stepDetails, key: key("enter"), want: stepPreset},
		{name: "analysis back to picker", from: stepDetails, key: key("esc"), want: stepPicking},
		{name: "preset to format", from: stepPreset, key: key("enter"), want: stepFormat},
		{name: "preset back to analysis", from: stepPreset, key: key("esc"), want: stepDetails},
		{name: "format to thumbnail question", from: stepFormat, key: key("enter"), want: stepThumbAsk},
		{name: "format back to preset", from: stepFormat, key: key("esc"), want: stepPreset},
		{name: "thumbnail question to position", from: stepThumbAsk, key: key("enter"), want: stepThumb},
		{name: "thumbnail question back to format", from: stepThumbAsk, key: key("esc"), want: stepFormat},
		{name: "position back to the question", from: stepThumb, key: key("esc"), want: stepThumbAsk},
		{name: "position to encoding", from: stepThumb, key: key("enter"), want: stepEncoding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := queued(t, false)
			m.step = tt.from

			if got := send(t, m, tt.key).step; got != tt.want {
				t.Errorf("step = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnsweredQuestionsAreSkipped(t *testing.T) {
	preset, _ := domain.PresetByLevel(domain.LevelAggressive)
	format, _ := domain.FormatByID("mp4-h264")
	thumb := domain.NoThumb()

	tests := []struct {
		name string
		cfg  func(*Config)
		from step
		want step
	}{
		{
			name: "preset given jumps to format",
			cfg:  func(c *Config) { c.Preset = &preset },
			from: stepDetails,
			want: stepFormat,
		},
		{
			name: "preset and format given jump to the thumbnail question",
			cfg:  func(c *Config) { c.Preset, c.Format = &preset, &format },
			from: stepDetails,
			want: stepThumbAsk,
		},
		{
			name: "everything given goes straight to encoding",
			cfg:  func(c *Config) { c.Preset, c.Format, c.Thumb = &preset, &format, &thumb },
			from: stepDetails,
			want: stepEncoding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := queued(t, false)
			tt.cfg(&m.cfg)
			m.step = tt.from

			if got := send(t, m, key("enter")).step; got != tt.want {
				t.Errorf("step = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecliningTheThumbnailSkipsThePositionStep(t *testing.T) {
	m := queued(t, false)
	m.step = stepThumbAsk

	m = send(t, m, key("n"))

	if m.step != stepEncoding {
		t.Errorf("step = %v, want stepEncoding", m.step)
	}
	if m.thumbSpec().Enabled {
		t.Error("thumbnail should be disabled after answering no")
	}
}

func TestAcceptingTheThumbnailAsksForThePosition(t *testing.T) {
	m := queued(t, false)
	m.step = stepThumbAsk

	m = send(t, m, key("y"))

	if m.step != stepThumb {
		t.Errorf("step = %v, want stepThumb", m.step)
	}
	if !m.thumbSpec().Enabled {
		t.Error("thumbnail should be enabled after answering yes")
	}
}

// --- measurement -------------------------------------------------------------

func TestSamplingWalksTheWholeQueue(t *testing.T) {
	m := newTestModel(t, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute))
	m.cursor = 0

	m = send(t, m, key("enter")) // queue both, start sampling
	if m.step != stepSampling {
		t.Fatalf("step = %v, want stepSampling", m.step)
	}

	m = send(t, m, sampledMsg{path: "a.mp4", complexity: testComplexity()})
	if m.step != stepSampling {
		t.Errorf("step = %v, want to keep sampling the second video", m.step)
	}

	m = send(t, m, sampledMsg{path: "b.mp4", complexity: testComplexity()})
	if m.step != stepDetails {
		t.Errorf("step = %v, want stepDetails once the queue is measured", m.step)
	}
	if !m.measured() {
		t.Error("every queued video should be measured")
	}
}

func TestSamplingFailureFallsBackToUpperBound(t *testing.T) {
	m := newTestModel(t)
	m = send(t, m, key("enter"))
	m = send(t, m, sampledMsg{path: "clip.mp4", err: errors.New("ffmpeg exploded")})

	if m.step != stepDetails {
		t.Errorf("step = %v, want stepDetails: a failed measurement is not fatal", m.step)
	}
	if m.measured() {
		t.Error("the queue must not be reported as measured")
	}
	if m.currentPlan().Measured {
		t.Error("plan must not claim to be measured")
	}
}

func TestMeasurementSharpensTheEstimate(t *testing.T) {
	m := newTestModel(t)
	m = send(t, m, key("enter"))

	before := m.currentPlan()
	m = send(t, m, sampledMsg{path: "clip.mp4", complexity: domain.Complexity{
		RefBitrate: 700_000, RefCRF: 33, RefPixels: 1920 * 1080, RefEfficiency: 1, Samples: 3,
	}})
	after := m.currentPlan()

	if !after.Measured {
		t.Fatal("plan should be flagged as measured")
	}
	if after.EstimatedBytes >= before.EstimatedBytes {
		t.Errorf("measured estimate %d should undercut the bound %d", after.EstimatedBytes, before.EstimatedBytes)
	}
}

func TestAnalysisCanBeSkipped(t *testing.T) {
	m := newTestModel(t)
	m.cfg.Analyze = false

	m = send(t, m, key("enter"))

	if m.step != stepDetails {
		t.Errorf("step = %v, want stepDetails without analysis", m.step)
	}
}

// --- thumbnail ---------------------------------------------------------------

func TestThumbnailTimestampControls(t *testing.T) {
	tests := []struct {
		name  string
		start time.Duration
		keys  []tea.KeyMsg
		want  time.Duration
	}{
		{name: "right adds a second", start: 30 * time.Second, keys: []tea.KeyMsg{key("right")}, want: 31 * time.Second},
		{name: "left removes a second", start: 30 * time.Second, keys: []tea.KeyMsg{key("left")}, want: 29 * time.Second},
		{name: "up adds ten seconds", start: 30 * time.Second, keys: []tea.KeyMsg{key("up")}, want: 40 * time.Second},
		{name: "down removes ten seconds", start: 30 * time.Second, keys: []tea.KeyMsg{key("down")}, want: 20 * time.Second},
		{name: "never goes below zero", start: 0, keys: []tea.KeyMsg{key("down")}, want: 0},
		{name: "never goes past the end", start: 5 * time.Minute, keys: []tea.KeyMsg{key("up")}, want: 5 * time.Minute},
		{name: "typing seconds", start: 0, keys: []tea.KeyMsg{key("9"), key("0")}, want: 90 * time.Second},
		{name: "typing mm:ss", start: 0, keys: []tea.KeyMsg{key("1"), key(":"), key("3"), key("0")}, want: 90 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := queued(t, false)
			m.step = stepThumb
			m.thumbAt = tt.start
			m.thumbBuf = ""

			for _, k := range tt.keys {
				m = send(t, m, k)
			}

			if m.thumbAt != tt.want {
				t.Errorf("thumbAt = %s, want %s", m.thumbAt, tt.want)
			}
		})
	}
}

func TestBatchThumbnailUsesPercentages(t *testing.T) {
	short := testVideo("short.mp4", 2*time.Minute)
	long := testVideo("long.mp4", 10*time.Minute)

	m := queued(t, true, short, long)
	m.step = stepThumb
	m.thumbPercent = 0.1

	m = send(t, m, key("up")) // +10 percentage points

	if m.thumbPercent != 0.2 {
		t.Fatalf("percent = %v, want 0.2", m.thumbPercent)
	}

	spec := m.thumbSpec()
	if got := spec.For(short.Info.Duration); got != 24*time.Second {
		t.Errorf("short video frame at %s, want 24s", got)
	}
	if got := spec.For(long.Info.Duration); got != 2*time.Minute {
		t.Errorf("long video frame at %s, want 2m", got)
	}
}

func TestQuitIsDisabledWhileTypingATimestamp(t *testing.T) {
	m := queued(t, false)
	m.step = stepThumb

	next, cmd := m.Update(key("q"))

	if next.(Model).step != stepThumb {
		t.Error("q must not quit while the timestamp field has focus")
	}
	if cmd != nil {
		t.Error("q must not emit a command while typing a timestamp")
	}
}

// --- encoding ----------------------------------------------------------------

func TestBatchEncodesEveryVideoInOrder(t *testing.T) {
	m := queued(t, true, testVideo("a.mp4", time.Minute), testVideo("b.mp4", time.Minute))
	m.step = stepThumb

	m = send(t, m, key("enter")) // start the first encode
	if m.step != stepEncoding || m.queueIndex != 0 {
		t.Fatalf("step = %v, queueIndex = %d, want encoding the first video", m.step, m.queueIndex)
	}

	m = send(t, m, encodeDoneMsg{res: app.Result{SourceBytes: 100, OutputBytes: 10}})
	if m.step != stepEncoding || m.queueIndex != 1 {
		t.Fatalf("step = %v, queueIndex = %d, want encoding the second video", m.step, m.queueIndex)
	}

	m = send(t, m, encodeDoneMsg{res: app.Result{SourceBytes: 100, OutputBytes: 10}})
	if m.step != stepDone {
		t.Errorf("step = %v, want stepDone once the queue is empty", m.step)
	}
	if len(m.results) != 2 {
		t.Errorf("kept %d results, want 2", len(m.results))
	}
}

func TestEncodeFailureIsSurfaced(t *testing.T) {
	m := queued(t, false)
	m = send(t, m, encodeFailedMsg{err: errors.New("codec not found")})

	if m.step != stepFatal {
		t.Errorf("step = %v, want stepFatal", m.step)
	}
	if m.err == nil {
		t.Error("the error must be kept so the view can show it")
	}
}

func TestOptimizerWritesTheArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		thumb     domain.ThumbSpec
		wantThumb bool
	}{
		{name: "with a thumbnail", thumb: domain.DefaultThumbSpec(), wantThumb: true},
		{name: "without a thumbnail", thumb: domain.NoThumb(), wantThumb: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()
			thumbs := &fakeThumbs{}
			video := testVideo("clip.mp4", 5*time.Minute)

			optimizer := app.Optimizer{Encoder: fakeEncoder{}, Thumbs: thumbs, OutDir: outDir, Report: true}
			res, err := optimizer.Run(context.Background(),
				domain.BuildPlan(video, domain.Presets[1], domain.DefaultFormat()), tt.thumb, nil)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if _, err := os.Stat(res.VideoPath); err != nil {
				t.Errorf("missing video: %v", err)
			}
			if _, err := os.Stat(res.ReportPath); err != nil {
				t.Errorf("missing report: %v", err)
			}
			if tt.wantThumb {
				if res.ThumbPath == "" || thumbs.grabbed != 1 {
					t.Error("expected a thumbnail to be captured")
				}
			} else if res.ThumbPath != "" || thumbs.grabbed != 0 {
				t.Error("no thumbnail should be captured when the spec is off")
			}
			if filepath.Ext(res.VideoPath) != ".webm" {
				t.Errorf("video extension = %q, want .webm", filepath.Ext(res.VideoPath))
			}
		})
	}
}

// --- rendering ---------------------------------------------------------------

func TestViewRendersEveryStep(t *testing.T) {
	steps := []step{
		stepScanning, stepPicking, stepSampling, stepDetails, stepPreset,
		stepFormat, stepThumbAsk, stepThumb, stepEncoding, stepDone, stepFatal,
	}

	for _, batch := range []bool{false, true} {
		for _, s := range steps {
			m := queued(t, batch, testVideo("a.mp4", time.Minute), testVideo("b.mp4", 2*time.Minute))
			m.plan = m.currentPlan()
			m.results = []app.Result{{Plan: m.plan, SourceBytes: 100, OutputBytes: 10, Reduction: 0.9}}
			m.err = errors.New("boom")
			m.step = s

			if out := m.View(); out == "" {
				t.Errorf("batch=%v step %v rendered nothing", batch, s)
			}
		}
	}
}
