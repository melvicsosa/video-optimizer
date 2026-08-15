# Contributing to vopt

Thanks for taking the time to contribute. This document explains how the
project is organised and what a good change looks like.

## Prerequisites

- Go 1.24+
- [ffmpeg](https://ffmpeg.org) (brings `ffprobe`) — `brew install ffmpeg`
- Optional, for the terminal frame preview: `chafa` or `viu`

## Getting started

```bash
git clone https://github.com/melvicsosa/video-optimizer
cd video-optimizer
make check   # vet, format check and the full test suite
make run     # build and launch the interactive flow
```

## Where things live

The code follows a hexagonal layout: the rules never import the tools.

| Package | Role | May import |
| --- | --- | --- |
| `internal/domain` | Compression rules, presets, formats, estimates | stdlib only |
| `internal/app` | The pipeline: encode → thumbnail → report | `domain` |
| `internal/infra/ffmpeg` | ffmpeg/ffprobe adapters | `domain` |
| `internal/infra/fsscan` | Directory scanning | `domain` |
| `internal/ui` | The Bubbletea interactive flow | `domain`, `app`, `infra` |
| `internal/humanize` | Byte/duration/percent formatting | stdlib only |
| `cmd/vopt` | Flags and wiring | everything |

A change that makes `internal/domain` import ffmpeg or Bubbletea will not be
merged — that separation is what keeps the rules testable without a video file
in sight.

## Common contributions

**Adding an output format** — add an entry to `domain.Formats` in
[internal/domain/plan.go](internal/domain/plan.go) and a matching case in
`ffmpeg.BuildArgs` in [internal/infra/ffmpeg/encoder.go](internal/infra/ffmpeg/encoder.go).
Add a table-driven case to the existing tests for both.

**Adding a preset** — one entry in `domain.Presets`. Presets set a CRF target
and a bitrate ceiling; the encoder runs in constrained-quality mode.

**UI changes** — the flow lives in `internal/ui/model.go` (state machine) and
`internal/ui/view.go` (rendering). Tests use table-driven state transitions;
follow the patterns in `model_test.go`.

## Quality bar

- `make check` must pass: `go vet`, `gofmt`, and the test suite.
- New behaviour ships with tests. Domain logic is tested without ffmpeg;
  adapters are tested against real binaries only when unavoidable.
- Comments explain **why**, not what. Exported symbols carry GoDoc comments.
- Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`).

## Reporting bugs

Open an issue with the vopt version (`vopt -version`), the ffmpeg version
(`ffmpeg -version | head -1`), the exact command, and what you expected versus
what happened. If the problem depends on the footage, `ffprobe` output for the
input file helps a lot.
