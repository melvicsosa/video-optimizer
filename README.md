<p align="center">
  <img src="assets/logo.png" alt="vopt — the video optimizer chameleon" width="420">
</p>

<h1 align="center">vopt · video optimizer</h1>

<p align="center">
  Compress videos with ffmpeg, capture their poster frame, and know the output size <em>before</em> you encode.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-38BDF8" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/go-1.24%2B-4ADE80" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-38BDF8" alt="Platform">
  <img src="https://img.shields.io/badge/built%20with-Bubbletea-4ADE80" alt="Built with Bubbletea">
</p>

---

## What it does

vopt is a terminal tool that walks you through compressing videos — which
files, how hard, which format, where the thumbnail goes — and every one of
those questions can be answered up front with a flag instead. Pass them all
plus `-y` and it becomes a scriptable batch encoder.

```
◆ video-optimizer
video › analysis › compression › format › thumbnail › encode

How hard should we compress?
3 videos · estimated with WebM · VP9

❯ Light        1.1 GB → ≈ 176.0 MB   −84%
    Keeps the source resolution. Visually identical, safest option.
    1920x1080 · CRF 30 · ~1.3 Mbps predicted

  Balanced     1.1 GB → ≈ 124.7 MB   −89%
    Keeps the source resolution. Best size/quality trade-off for the web.
    1920x1080 · CRF 33 · ~900 kbps predicted

  Aggressive   1.1 GB → ≈ 53.4 MB    −95%
    Downscales to 720p. Smallest file, ideal for previews and mobile.
    1280x720 · CRF 36 · ~346 kbps predicted
```

> [!NOTE]
> **Why the estimates are honest.** Most compression tools quote a bitrate
> ceiling and hope for the best. vopt encodes three two-second excerpts of
> your footage first, measures what came out and extrapolates from there. A
> static talking head and a handheld action shot answer the same settings
> very differently, and two seconds of CPU is a cheap price for knowing which
> one you have. An estimate marked `≈` was measured; one marked `≤` is an
> upper bound, shown when the measurement failed or `-no-analyze` was passed.

## Quick start

### Install

Requires [ffmpeg](https://ffmpeg.org) (which brings `ffprobe`) and Go 1.24+.

```bash
brew install ffmpeg
```

```bash
go install github.com/melvicsosa/video-optimizer/cmd/vopt@latest
```

Or from a clone:

```bash
make install
```

### Run

Run it inside a folder with videos:

```bash
vopt
```

Pick one video or the **All videos** entry to process the whole folder one
file after another. Encoded files, thumbnails and a small JSON report land in
`output/`.

### The flow at a glance

```mermaid
flowchart LR
    pick["pick videos"] --> analyze["measure samples"]
    analyze --> preset["choose preset"]
    preset --> format["choose format"]
    format --> thumb["place thumbnail"]
    thumb --> encode["encode + report"]
```

Any flag you pass is a question the flow will not ask:

```bash
vopt -all -preset aggressive
```

That still asks for the format and the thumbnail, but nothing else. Add `-y`
to skip the interface entirely:

```bash
vopt -y -all -preset aggressive -format webm-av1 -thumb 25%
vopt -y -input talk.mov -preset light -no-thumb
```

## Reference

### Flags

| Flag | Default | What it does |
| --- | --- | --- |
| `-dir` | `.` | Directory to scan |
| `-out` | `output` | Where encoded files land |
| `-input` | | Process one file only |
| `-all` | | Process every video in the directory |
| `-preset` | `balanced` | `light`, `balanced` or `aggressive` |
| `-format` | `webm-vp9` | `webm-vp9`, `webm-av1`, `mp4-h265`, `mp4-h264` |
| `-thumb` | `10%` | `off`, a percentage, `12` or `1:30` |
| `-no-thumb` | | Same as `-thumb off` |
| `-keep-names` | | Keep original names instead of web friendly slugs |
| `-no-report` | | Skip the JSON sidecar |
| `-no-analyze` | | Skip the measurement, fall back to upper bounds |
| `-y` | | Run without asking anything |
| `-version` | | Print the version and exit |

### Presets

| Preset | Resolution | Aimed at |
| --- | --- | --- |
| Light | unchanged | Archival copies that must stay visually identical |
| Balanced | unchanged | The default for web delivery |
| Aggressive | 720p | Previews, mobile, embedded players |

Presets set a quality target (CRF) and a bitrate ceiling. The encoder runs in
constrained-quality mode: it spends what the footage needs and never crosses
the ceiling, so simple footage lands far below it.

### Formats

| Format | Notes |
| --- | --- |
| WebM · VP9 | Universal browser support. The safe default. |
| WebM · AV1 | Smallest files, faster to encode than VP9. Safari 17+. |
| MP4 · H.265 | Great on Apple devices. Not supported by Firefox. |
| MP4 · H.264 | Plays everywhere. Largest output. |

### Thumbnails

In a single-video run you pick an exact timestamp, nudging it with the arrow
keys and previewing the frame with `p` (uses `chafa` or `viu` when installed,
otherwise the system image viewer). In a batch the position is a percentage
of each video's duration, since the files have different lengths.

### Output

```
output/
  microcurso-indicadores.webm
  microcurso-indicadores-thumb.jpg
  microcurso-indicadores.json
```

The JSON report carries what a web player needs: dimensions, duration, poster
frame and how much weight was removed.

## Documentation

| Your task | Start here |
| --- | --- |
| Understand how the pieces fit | [docs/architecture.md](docs/architecture.md) |
| Add a format, preset or UI change | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Hack on the code | `make check` — vet, format check and the test suite |

The code is organised so the rules stay separate from the tools:

```
internal/domain   compression rules, estimates, formats — no ffmpeg, no terminal
internal/app      the pipeline: encode, capture, report
internal/infra    adapters: ffprobe, ffmpeg, the filesystem
internal/ui       the interactive flow
cmd/vopt          flags and wiring
```

## Contributing

Bug reports, formats and presets are all welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md) for the layout, the quality bar and the
two-file recipe for adding an output format.

## License

[MIT](LICENSE)
