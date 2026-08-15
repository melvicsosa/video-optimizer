package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/melvicsosa/video-optimizer/internal/domain"
	"github.com/melvicsosa/video-optimizer/internal/humanize"
)

// View renders the current step framed by a shared header and help line.
func (m Model) View() string {
	var body, help string

	switch m.step {
	case stepScanning:
		body, help = m.viewScanning()
	case stepPicking:
		body, help = m.viewPicking()
	case stepSampling:
		body, help = m.viewSampling()
	case stepDetails:
		body, help = m.viewDetails()
	case stepPreset:
		body, help = m.viewPreset()
	case stepFormat:
		body, help = m.viewFormat()
	case stepThumbAsk:
		body, help = m.viewThumbAsk()
	case stepThumb:
		body, help = m.viewThumb()
	case stepEncoding:
		body, help = m.viewEncoding()
	case stepDone:
		body, help = m.viewDone()
	case stepFatal:
		body, help = m.viewFatal()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		m.divider(),
		"",
		body,
		"",
		"  "+help,
		"",
	)
}

// helpLine renders key/action pairs: the key in bold, its action faint, so the
// pressable part reads at a glance.
func helpLine(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, styleKey.Render(pairs[i])+" "+styleHelp.Render(pairs[i+1]))
	}
	return strings.Join(parts, styleHelp.Render("  ·  "))
}

// divider draws a rule between the header and the content, sweeping the same
// green-to-blue gradient as the rest of the brand.
func (m Model) divider() string {
	return "\n  " + pixels(strings.Repeat("─", clamp(m.width-4, 20, 72)))
}

// header keeps the banner on every screen, so the content below always starts
// at the same position; only the breadcrumb trail changes per step.
func (m Model) header() string {
	if m.step == stepScanning || m.step == stepFatal {
		return m.logo()
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.logo(), "", m.crumbs())
}

// crumbs renders the step trail with the active step highlighted.
func (m Model) crumbs() string {
	labels := []string{"video", "analysis", "compression", "format", "thumbnail", "encode"}
	current := map[step]int{
		stepPicking: 0, stepSampling: 1, stepDetails: 1, stepPreset: 2,
		stepFormat: 3, stepThumbAsk: 4, stepThumb: 4, stepEncoding: 5, stepDone: 5,
	}[m.step]

	var crumbs []string
	for i, label := range labels {
		if i == current {
			crumbs = append(crumbs, styleAccent.Render(label))
			continue
		}
		crumbs = append(crumbs, styleFaint.Render(label))
	}
	return styleStep.Render("  " + strings.Join(crumbs, styleFaint.Render(" › ")))
}

func (m Model) viewScanning() (string, string) {
	return fmt.Sprintf("  %s scanning %s", m.spinner.View(), styleMuted.Render(m.cfg.Dir)),
		helpLine("ctrl+c", "quit")
}

func (m Model) viewPicking() (string, string) {
	var b strings.Builder
	b.WriteString(styleHeading.Render("  What should we compress?") + "\n\n")

	row := 0
	if len(m.videos) > 1 {
		var total int64
		for _, v := range m.videos {
			total += v.SizeBytes
		}
		label := fmt.Sprintf("All videos (%d)", len(m.videos))
		if m.batchRow() {
			label = styleSelected.Render(label)
		}
		b.WriteString("  " + cursor(m.batchRow()) + label + "\n")
		b.WriteString("      " + styleFaint.Render(humanize.Bytes(total)+" total · encoded one after another") + "\n\n")
		row++
	}

	for i, v := range m.videos {
		selected := m.cursor == row+i
		name := v.FileName()
		if selected {
			name = styleSelected.Render(name)
		} else {
			name = styleText.Render(name)
		}
		b.WriteString("  " + cursor(selected) + name + "\n")

		meta := "unreadable file, ffprobe could not open it"
		if v.Probed {
			meta = fmt.Sprintf("%s · %s · %s · %s",
				humanize.Bytes(v.SizeBytes),
				humanize.Duration(v.Info.Duration),
				v.Info.ResolutionLabel(),
				humanize.Bitrate(v.Info.EffectiveVideoBitrate()),
			)
		}
		b.WriteString("      " + styleFaint.Render(meta) + "\n")
		if i < len(m.videos)-1 {
			b.WriteString("\n")
		}
	}

	help := helpLine("↑/↓", "move", "enter", "select", "q", "quit")
	if len(m.videos) > 1 {
		help = helpLine("↑/↓", "move", "enter", "select", "a", "all", "q", "quit")
	}
	return b.String(), help
}

func (m Model) viewSampling() (string, string) {
	scope := m.reference().FileName()
	if m.batch {
		scope = fmt.Sprintf("video %d of %d", min(m.sampleIndex+1, len(m.queue)), len(m.queue))
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		styleHeading.Render("  Analysis"),
		"",
		fmt.Sprintf("  %s measuring how demanding this footage is %s",
			m.spinner.View(), styleFaint.Render("— "+scope)),
		"  "+styleFaint.Render("encoding three short excerpts to predict the real output size"),
	)
	return body, helpLine("ctrl+c", "quit")
}

func (m Model) viewDetails() (string, string) {
	if m.batch {
		return m.viewBatchDetails()
	}

	video := m.reference()
	info := video.Info
	rows := [][2]string{
		{"File", video.FileName()},
		{"Size", humanize.Bytes(video.SizeBytes)},
		{"Duration", humanize.Duration(info.Duration)},
		{"Resolution", fmt.Sprintf("%s  (%s)", info.Resolution(), info.ResolutionLabel())},
		{"Frame rate", fmt.Sprintf("%.2f fps", info.FPS)},
		{"Video", fmt.Sprintf("%s at %s", info.VideoCodec, humanize.Bitrate(info.EffectiveVideoBitrate()))},
	}
	if info.HasAudio {
		rows = append(rows, [2]string{"Audio", fmt.Sprintf("%s at %s", info.AudioCodec, humanize.Bitrate(info.AudioBitrate))})
	} else {
		rows = append(rows, [2]string{"Audio", "none"})
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{
			styleHeading.Render("  Analysis"),
			"",
			stylePanelSource.Render(renderRows(rows)),
			"",
			"  " + styleMuted.Render(verdict(info)),
		}, m.measurementNote()...)...,
	)
	return body, helpLine("enter", "continue", "esc", "back", "q", "quit")
}

func (m Model) viewBatchDetails() (string, string) {
	var (
		b        strings.Builder
		total    int64
		duration time.Duration
	)
	b.WriteString(styleHeading.Render(fmt.Sprintf("  Analysis — %d videos", len(m.queue))) + "\n\n")

	for _, v := range m.queue {
		total += v.SizeBytes
		duration += v.Info.Duration

		b.WriteString("  " + styleText.Render(v.FileName()) + "\n")
		measured := ""
		if c := m.complexityOf(v); c.Valid() {
			measured = " · measured " + humanize.Bitrate(c.RefBitrate)
		}
		b.WriteString("      " + styleFaint.Render(fmt.Sprintf("%s · %s · %s%s",
			humanize.Bytes(v.SizeBytes),
			humanize.Duration(v.Info.Duration),
			v.Info.ResolutionLabel(),
			measured,
		)) + "\n")
	}

	b.WriteString("\n" + stylePanelSource.Render(renderRows([][2]string{
		{"Videos", fmt.Sprint(len(m.queue))},
		{"Total size", humanize.Bytes(total)},
		{"Total runtime", humanize.Duration(duration)},
	})))

	body := lipgloss.JoinVertical(lipgloss.Left, append([]string{b.String()}, m.measurementNote()...)...)
	return body, helpLine("enter", "continue", "esc", "back", "q", "quit")
}

// measurementNote explains whether the estimates are predictions or bounds.
func (m Model) measurementNote() []string {
	switch {
	case !m.cfg.Analyze:
		return []string{"", "  " + styleFaint.Render("analysis skipped, estimates below are upper bounds")}
	case m.measured():
		c := m.complexityOf(m.reference())
		return []string{"", "  " + styleFaint.Render(fmt.Sprintf(
			"measured on %d excerpts: %s at CRF %d — estimates below are predictions, not upper bounds",
			c.Samples, humanize.Bitrate(c.RefBitrate), c.RefCRF))}
	case m.sampleErr != nil:
		return []string{"", "  " + styleWarn.Render("could not measure every source, some estimates are upper bounds")}
	default:
		return nil
	}
}

func (m Model) viewPreset() (string, string) {
	var b strings.Builder
	b.WriteString(styleHeading.Render("  How hard should we compress?") + "\n")
	b.WriteString("  " + styleFaint.Render(m.scopeLabel()+" · estimated with "+m.format().Name) + "\n\n")

	for i, preset := range domain.Presets {
		source, estimated, measured := m.queueTotals(preset, m.format())
		reduction := reductionOf(source, estimated)
		selected := i == m.presetCursor

		name := preset.Name
		if selected {
			name = styleSelected.Render(name)
		}
		b.WriteString(fmt.Sprintf("  %s%s   %s → %s  %s\n",
			cursor(selected), name,
			styleFaint.Render(humanize.Bytes(source)),
			styleAccent2.Render(prefix(measured)+humanize.Bytes(estimated)),
			styleOK.Render("−"+humanize.Percent(reduction)),
		))
		b.WriteString("      " + styleFaint.Render(preset.Summary) + "\n")

		plan := m.planFor(m.reference(), preset, m.format())
		b.WriteString("      " + styleFaint.Render(fmt.Sprintf(
			"%s · CRF %d · %s", resolutionOf(plan), plan.CRF, bitrateNote(plan))) + "\n")
		if i < len(domain.Presets)-1 {
			b.WriteString("\n")
		}
	}

	return b.String(), helpLine("↑/↓", "move", "enter", "select", "esc", "back", "q", "quit")
}

func (m Model) viewFormat() (string, string) {
	var b strings.Builder
	b.WriteString(styleHeading.Render("  Output format") + "\n")
	b.WriteString("  " + styleFaint.Render(m.scopeLabel()+" · preset: "+m.preset().Name) + "\n\n")

	for i, format := range domain.Formats {
		source, estimated, measured := m.queueTotals(m.preset(), format)
		selected := i == m.formatCursor

		name := format.Name
		if selected {
			name = styleSelected.Render(name)
		}
		b.WriteString(fmt.Sprintf("  %s%s   %s  %s\n",
			cursor(selected), name,
			styleAccent2.Render(prefix(measured)+humanize.Bytes(estimated)),
			styleOK.Render("−"+humanize.Percent(reductionOf(source, estimated))),
		))
		b.WriteString("      " + styleFaint.Render(format.Note) + "\n")
		if i < len(domain.Formats)-1 {
			b.WriteString("\n")
		}
	}

	return b.String(), helpLine("↑/↓", "move", "enter", "select", "esc", "back", "q", "quit")
}

func (m Model) viewThumbAsk() (string, string) {
	options := []struct {
		label string
		note  string
		yes   bool
	}{
		{label: "Yes, capture a poster frame", note: "saved as a JPEG next to each video", yes: true},
		{label: "No, video only", note: "skips the capture step entirely", yes: false},
	}

	var b strings.Builder
	b.WriteString(styleHeading.Render("  Do you want a thumbnail?") + "\n")
	b.WriteString("  " + styleFaint.Render(m.scopeLabel()) + "\n\n")

	for _, option := range options {
		selected := option.yes == m.thumbAskYes
		label := option.label
		if selected {
			label = styleSelected.Render(label)
		}
		b.WriteString("  " + cursor(selected) + label + "\n")
		b.WriteString("      " + styleFaint.Render(option.note) + "\n\n")
	}

	return b.String(), helpLine("↑/↓", "move", "enter", "select", "y", "yes", "n", "no", "esc", "back")
}

func (m Model) viewThumb() (string, string) {
	if m.batch {
		return m.viewThumbBatch()
	}

	video := m.reference()
	total := video.Info.Duration
	ratio := 0.0
	if total > 0 {
		ratio = m.thumbAt.Seconds() / total.Seconds()
	}

	var b strings.Builder
	b.WriteString(styleHeading.Render("  Thumbnail position") + "\n")
	b.WriteString("  " + styleFaint.Render("pick the frame used as poster image") + "\n\n")
	b.WriteString("  " + styleAccent.Render(humanize.Duration(m.thumbAt)) +
		styleFaint.Render(" / "+humanize.Duration(total)) + "\n\n")
	b.WriteString("  " + timeline(ratio, clamp(m.width-8, 20, 62)) + "\n\n")
	b.WriteString("  " + styleMuted.Render("edit: ") + styleText.Render(m.thumbBuf) + styleAccent.Render("▏") + "\n")
	b.WriteString(m.previewNote())
	b.WriteString("\n" + stylePanelTarget.Render(renderRows(m.planSummaryRows())))

	return b.String(), helpLine("←/→", "±1s", "↑/↓", "±10s", "type", "1:30", "p", "preview", "enter", "encode", "esc", "back")
}

func (m Model) viewThumbBatch() (string, string) {
	var b strings.Builder
	b.WriteString(styleHeading.Render("  Thumbnail position") + "\n")
	b.WriteString("  " + styleFaint.Render("videos have different lengths, so the frame is placed by percentage") + "\n\n")
	b.WriteString("  " + styleAccent.Render(fmt.Sprintf("%.0f%%", m.thumbPercent*100)) +
		styleFaint.Render(" into each video") + "\n\n")
	b.WriteString("  " + timeline(m.thumbPercent, clamp(m.width-8, 20, 62)) + "\n\n")

	shown := m.queue
	if len(shown) > 3 {
		shown = shown[:3]
	}
	for _, v := range shown {
		at := m.thumbSpec().For(v.Info.Duration)
		b.WriteString("  " + styleFaint.Render(fmt.Sprintf("%-28s → %s",
			truncate(v.FileName(), 28), humanize.Duration(at))) + "\n")
	}
	if len(m.queue) > len(shown) {
		b.WriteString("  " + styleFaint.Render(fmt.Sprintf("… and %d more", len(m.queue)-len(shown))) + "\n")
	}

	b.WriteString("\n  " + styleMuted.Render("edit: ") + styleText.Render(m.thumbBuf) + styleAccent.Render("▏%") + "\n")
	b.WriteString(m.previewNote())
	b.WriteString("\n" + stylePanelTarget.Render(renderRows(m.planSummaryRows())))

	return b.String(), helpLine("←/→", "±1%", "↑/↓", "±10%", "type", "25", "p", "preview", "enter", "encode", "esc", "back")
}

func (m Model) previewNote() string {
	switch {
	case m.previewErr != nil:
		return "\n  " + styleWarn.Render("preview unavailable: "+m.previewErr.Error()) + "\n"
	case m.previewPath != "":
		return "\n  " + styleFaint.Render("last preview: "+m.previewPath) + "\n"
	default:
		return ""
	}
}

func (m Model) planSummaryRows() [][2]string {
	source, estimated, measured := m.queueTotals(m.preset(), m.format())
	plan := m.planFor(m.reference(), m.preset(), m.format())

	return [][2]string{
		{"Output", fmt.Sprintf("%s · %s · %s", m.format().Name, resolutionOf(plan), m.preset().Name)},
		{"Target", fmt.Sprintf("%s%s  (−%s)", prefix(measured), humanize.Bytes(estimated), humanize.Percent(reductionOf(source, estimated)))},
	}
}

func (m Model) viewEncoding() (string, string) {
	video := m.current()
	total := video.Info.Duration

	percent := 0.0
	if total > 0 {
		percent = clampFloat(m.progress.Elapsed.Seconds()/total.Seconds(), 0, 1)
	}

	eta := "estimating"
	if m.progress.Speed > 0 && total > m.progress.Elapsed {
		remaining := time.Duration(float64(total-m.progress.Elapsed) / m.progress.Speed)
		eta = humanize.Duration(remaining) + " left"
	}

	heading := "  Encoding"
	if m.batch {
		heading = fmt.Sprintf("  Encoding %d of %d", m.queueIndex+1, len(m.queue))
	}

	var b strings.Builder
	b.WriteString(styleHeading.Render(heading) + "\n")
	b.WriteString("  " + styleFaint.Render(video.FileName()) + "\n\n")
	b.WriteString("  " + m.bar.ViewAs(percent) + "  " + styleAccent2.Render(fmt.Sprintf("%3.0f%%", percent*100)) + "\n\n")
	b.WriteString("  " + styleMuted.Render(fmt.Sprintf(
		"%s / %s · %.1fx realtime · %.0f fps · %s",
		humanize.Duration(m.progress.Elapsed), humanize.Duration(total),
		m.progress.Speed, m.progress.FPS, eta,
	)) + "\n\n")
	b.WriteString("  " + m.spinner.View() + styleFaint.Render(fmt.Sprintf(" %s · CRF %d · ceiling %s",
		m.plan.Format.VideoEncoder, m.plan.CRF, humanize.Bitrate(m.plan.VideoBitrate))))

	if m.batch && len(m.results) > 0 {
		var saved int64
		for _, r := range m.results {
			saved += r.SourceBytes - r.OutputBytes
		}
		b.WriteString("\n\n  " + styleOK.Render(fmt.Sprintf("%d done · %s saved so far", len(m.results), humanize.Bytes(saved))))
	}

	return b.String(), helpLine("esc", "cancel")
}

func (m Model) viewDone() (string, string) {
	var source, output int64
	var took time.Duration
	for _, r := range m.results {
		source += r.SourceBytes
		output += r.OutputBytes
		took += r.Took
	}
	reduction := reductionOf(source, output)

	headline := styleOK.Render(fmt.Sprintf("  ✓ Done — %s smaller", humanize.Percent(reduction)))
	if reduction < 0.5 {
		headline = styleWarn.Render(fmt.Sprintf("  ✓ Done — %s smaller", humanize.Percent(reduction)))
	}

	var b strings.Builder
	b.WriteString(headline + "\n\n")

	if len(m.results) == 1 {
		res := m.results[0]
		rows := [][2]string{{"Video", res.VideoPath}}
		if res.ThumbPath != "" {
			rows = append(rows, [2]string{"Thumbnail", res.ThumbPath})
		}
		if res.ReportPath != "" {
			rows = append(rows, [2]string{"Report", res.ReportPath})
		}
		rows = append(rows,
			[2]string{"Size", fmt.Sprintf("%s → %s", humanize.Bytes(source), humanize.Bytes(output))},
			[2]string{"Saved", humanize.Bytes(source - output)},
			[2]string{"Took", humanize.Duration(took)},
		)
		b.WriteString(stylePanelTarget.Render(renderRows(rows)))
	} else {
		for _, r := range m.results {
			b.WriteString("  " + styleText.Render(truncate(r.Plan.Source.FileName(), 34)) + "  " +
				styleFaint.Render(humanize.Bytes(r.SourceBytes)+" → ") +
				styleAccent2.Render(humanize.Bytes(r.OutputBytes)) + "  " +
				styleOK.Render("−"+humanize.Percent(r.Reduction)) + "\n")
		}
		b.WriteString("\n" + stylePanelTarget.Render(renderRows([][2]string{
			{"Folder", m.opt.OutDir},
			{"Videos", fmt.Sprint(len(m.results))},
			{"Size", fmt.Sprintf("%s → %s", humanize.Bytes(source), humanize.Bytes(output))},
			{"Saved", humanize.Bytes(source - output)},
			{"Took", humanize.Duration(took)},
		})))
	}

	help := helpLine("o", "open folder", "n", "another video", "enter", "quit")
	if m.cfg.Target != TargetAsk {
		help = helpLine("o", "open folder", "enter", "quit")
	}
	return b.String(), help
}

func (m Model) viewFatal() (string, string) {
	msg := "unknown error"
	if m.err != nil {
		msg = m.err.Error()
	}
	return "  " + styleErr.Render("✗ ") + styleText.Render(msg), helpLine("enter", "quit")
}

// scopeLabel describes what the choice applies to.
func (m Model) scopeLabel() string {
	if m.batch {
		return fmt.Sprintf("%d videos", len(m.queue))
	}
	return m.reference().FileName()
}

// prefix distinguishes a measured prediction from a plain upper bound.
func prefix(measured bool) string {
	if measured {
		return "≈ "
	}
	return "≤ "
}

// bitrateNote explains which number drives the estimate.
func bitrateNote(plan domain.EncodePlan) string {
	if plan.Measured && plan.PredictedBitrate < plan.VideoBitrate {
		return "~" + humanize.Bitrate(plan.PredictedBitrate) + " predicted"
	}
	return humanize.Bitrate(plan.VideoBitrate) + " ceiling"
}

func reductionOf(source, output int64) float64 {
	if source <= 0 || output <= 0 {
		return 0
	}
	return 1 - float64(output)/float64(source)
}

// verdict tells the user, in plain words, how much room there is to compress.
func verdict(info domain.MediaInfo) string {
	perPixel := 0.0
	if info.Width > 0 && info.Height > 0 && info.FPS > 0 {
		perPixel = float64(info.EffectiveVideoBitrate()) / (float64(info.Width) * float64(info.Height) * info.FPS)
	}
	switch {
	case perPixel >= 0.12:
		return "This file is heavily over-encoded. Expect large savings with no visible loss."
	case perPixel >= 0.06:
		return "Comfortable headroom: it can lose a lot of weight before quality suffers."
	case perPixel >= 0.03:
		return "Already reasonably encoded. Prefer the light or balanced preset."
	default:
		return "Already tightly encoded. Aggressive settings may soften the image."
	}
}

// renderRows aligns a label/value table.
func renderRows(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(styleMuted.Render(fmt.Sprintf("%-*s", width, r[0])) + "  " + styleText.Render(r[1]))
	}
	return b.String()
}

// timeline draws the scrubber used to place the poster frame.
func timeline(ratio float64, width int) string {
	if width < 4 {
		width = 4
	}
	pos := clamp(int(ratio*float64(width-1)), 0, width-1)
	return styleAccent.Render(strings.Repeat("─", pos)) +
		styleSelected.Render("◆") +
		styleFaint.Render(strings.Repeat("─", width-1-pos))
}

func resolutionOf(plan domain.EncodePlan) string {
	return fmt.Sprintf("%dx%d", plan.OutputWidth(), plan.OutputHeight())
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
