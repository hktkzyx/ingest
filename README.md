# ingest

> 智能素材导入工具 — 插卡 → 自动识别设备 → 推断时间段 → 模板归档 → 校验通过 → 安全完成。

`ingest` is a cross-platform CLI for offloading footage from camera SD cards to a structured archive, with byte-exact verification and idempotent re-runs. It targets multi-device creators (mirrorless + drone + action cam) who want the reliability of `rsync` plus the convenience of commercial tools like Kocard / Hedge — without the lock-in.

**Status**: `v0.0.1-dev` — Phase 1 MVP. Engine works end-to-end via CLI flags. TUI, EXIF, auto mount detection, and YAML config are on the roadmap. See [PRD.md](./PRD.md) for the full specification.

---

## What it does

- **Auto-detect** the source device (Sony, DJI Pocket3 built-in; YAML rules planned)
- **Infer date range** from file mtime (EXIF / QuickTime extraction planned)
- **Render target paths** from a configurable template
- **Safe-copy** with streaming xxHash64, atomic temp-file rename, and mtime/permission preservation
- **Idempotent**: re-running on the same card skips files already verified in the SQLite history DB

---

## Install

### Prerequisites

- Go **1.24+** (the build will auto-bootstrap to the toolchain version pinned in `go.mod`)

### From source

```bash
git clone https://github.com/hktkzyx/ingest.git
cd ingest
go install ./cmd/ingest
```

The binary lands in `$(go env GOBIN)` (default `~/go/bin`). Add it to your `PATH`:

```bash
export PATH="$HOME/go/bin:$PATH"   # add to ~/.zshrc or ~/.bashrc
```

### Verify

```bash
ingest version          # → ingest 0.0.1-dev
ingest devices list     # → built-in device rules
```

---

## Quick start

```bash
# Preview only — see what would be copied where
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "周末骑行" --dry-run

# Real run, verbose
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "周末骑行" -v
```

Resulting layout:

```
~/Backups/
└── 20260427-周末骑行/
    └── origin-zve10m2/
        ├── C0001.MP4
        ├── C0002.MP4
        └── DSC00001.ARW
```

Re-run the same command and every file will skip in milliseconds — the history DB at `~/.local/share/ingest/ingest.db` remembers what's been ingested.

---

## Commands

| Command | Description |
|---|---|
| `ingest [flags]` | Run an ingestion (default command) |
| `ingest version` | Show version |
| `ingest devices list` | List built-in device detection rules |

### Flags

| Flag | Default | Description |
|---|---|---|
| `-s`, `--source` | _(required)_ | Source path (mounted volume / directory) |
| `-t`, `--target` | `~/Backups` | Target root directory |
| `-n`, `--name` | _(required)_ | Event name, used in `{event_name}` |
| `--device` | _(auto-detect)_ | Force a device id, e.g. `zve10m2`, `pocket3` |
| `--from` | _(from mtime)_ | Period start (`YYYY-MM-DD`) |
| `--to` | _(from mtime)_ | Period end (`YYYY-MM-DD`) |
| `--template` | `{date_start}[_{date_end}]-{event_name}/origin-{device_id}` | Path template |
| `--db` | `~/.local/share/ingest/ingest.db` | History database path |
| `--dry-run` | `false` | Preview, do not copy |
| `-v`, `--verbose` | `false` | Verbose per-file output |

### Template syntax

- `{var}` — variable substitution (errors if undefined)
- `[ ... ]` — optional segment, omitted entirely if any contained `{var}` is empty

| Variable | Example |
|---|---|
| `date_start` | `20260427` |
| `date_end` | `20260503` (empty for single-day shoots) |
| `event_name` | `周末骑行` |
| `device_id` | `zve10m2` |
| `device_name` | `Sony ZV-E10M2` |

Single-day: `{date_start}[_{date_end}]-{event_name}/origin-{device_id}` → `20260427-周末骑行/origin-zve10m2`
Multi-day: same template → `20260427_20260503-五一假期/origin-pocket3`

---

## How safe-copy works

For every file, `ingest` follows the [§FR-005 protocol](./PRD.md#fr-005-安全拷贝协议):

1. **Incremental check** — if the target exists at the same size and the history DB has a matching record, only the source is re-hashed; matching hash means skip.
2. **Stream copy** — read source once; `io.MultiWriter` simultaneously writes to a temp file (`*.ingest.tmp.<rand>`) and feeds an xxHash64 of the source bytes.
3. **Disk re-verify** — re-read the temp file from disk, compute its xxHash64. Mismatch → delete temp and report failure.
4. **Atomic rename** — `os.Rename(tmp, dst)` makes the file visible only after successful verification.
5. **Metadata restore** — apply source mtime and permissions to the new file.
6. **History record** — write `{source, target, device, size, hash}` to SQLite.

Any interruption — power loss, cable yank, disk full — leaves either the original temp file (which `ingest` will retry on the next run) or a fully-verified final file. There is no in-between state.

---

## Project layout

```
.
├── PRD.md                  # Canonical product spec — start here for design intent
├── README.md               # This file
├── CONTRIBUTING.md         # Dev setup, branching, code style
├── cmd/ingest/main.go      # CLI entry: flag parsing + orchestration only
├── internal/
│   ├── scanner/            # Walks source volume, classifies media vs sidecar
│   ├── device/             # Device detection rules + matcher
│   ├── period/             # Date-range inference (mtime; EXIF planned)
│   ├── template/           # Path template parser/renderer
│   ├── copier/             # Safe-copy protocol — the critical path
│   └── db/                 # SQLite history (modernc.org/sqlite, pure Go)
├── go.mod / go.sum
```

---

## For AI agents

If you are an AI coding agent working on this repo, here is the minimum context to be productive:

- **Authoritative spec**: [`PRD.md`](./PRD.md). Anything in this README is a summary; the PRD wins on conflicts.
- **No public Go API yet**: `internal/` is intentionally non-importable. The only stable surface is the CLI.
- **Branching**: this repo uses **gitflow**. Never commit to `main` or `develop`; open `feature/<name>` off `develop`. Full rules in [`CONTRIBUTING.md`](./CONTRIBUTING.md).
- **Verification commands** before declaring a task done:
  ```bash
  go vet ./...
  go build ./...
  go test ./...        # tests scaffolded in Phase 4; absence is expected today
  ```
- **Golden path test** (no real SD card needed):
  ```bash
  mkdir -p /tmp/fake-sony/PRIVATE/SONY /tmp/fake-sony/DCIM/100MSDCF
  echo hello > /tmp/fake-sony/DCIM/100MSDCF/C0001.MP4
  go run ./cmd/ingest --source /tmp/fake-sony --target /tmp/out --name test -v
  ```
  Expected: device detected as `zve10m2`, file copied, hash recorded, exit 0.
- **Critical files to read before changing**:
  - `internal/copier/copier.go` — safe-copy invariants; do not relax the verification step
  - `internal/db/db.go` — schema is `UNIQUE(target_path)`; migrations are not yet in place
  - `cmd/ingest/main.go` — all CLI surface
- **Out of scope** without explicit instruction: TUI, EXIF parsing, network I/O, YAML config — these are on the roadmap (PRD §11) but not yet wired in.

---

## Roadmap

See [PRD.md §11](./PRD.md#11-里程碑规划). Highlights:

| Version | Focus |
|---|---|
| **v0.1.0** | TUI prompts, EXIF date extraction, auto mount detection, cross-platform release |
| **v0.2.0** | YAML config, multi-target backup, `verify` / `history` subcommands |
| **v0.3.0** | Proxy generation (FFmpeg), multi-card queue, NLE XML export, cloud archive |
| **v1.0.0** | >80% test coverage, package distribution (Homebrew/Scoop) |

---

## License

TBD — likely MIT or Apache-2.0. Pending choice.
