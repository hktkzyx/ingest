# Contributing to ingest

Thanks for your interest. This document covers everything a developer needs to start contributing: environment setup, project layout, branching, commits, code style, and the release process.

---

## Development environment

### Prerequisites

- **Go 1.24+** — the build auto-bootstraps to the toolchain version pinned in `go.mod` (currently 1.25.0)
- **git 2.x** with gitflow conventions (see [Branching](#branching-gitflow))
- Optional: `golangci-lint`, `make`

### Bootstrap

```bash
git clone https://github.com/hktkzyx/ingest.git
cd ingest
go mod download
go build ./...
go vet ./...
```

### Run locally without installing

```bash
go run ./cmd/ingest --source /tmp/fake-card --target /tmp/out --name test --dry-run
```

### Local install for dogfooding

```bash
go install ./cmd/ingest
# binary at $(go env GOBIN), default ~/go/bin
```

---

## Project layout

| Path | Responsibility |
|---|---|
| `cmd/ingest/` | CLI entry — flag parsing and orchestration only. **No business logic.** |
| `internal/scanner/` | Walks the source volume, classifies media vs sidecar files. |
| `internal/device/` | Device detection rules and matcher. New built-in devices go here. |
| `internal/period/` | Date-range inference. Currently mtime-based; pluggable for EXIF. |
| `internal/template/` | Path template parser and renderer. |
| `internal/copier/` | Safe-copy protocol — the critical path. **Touch with care.** |
| `internal/db/` | SQLite history. Schema lives in `db.go` constant `schema`. |
| `PRD.md` | Authoritative product specification. |

**Dependency rule**: `cmd/ingest` may import any `internal/*`; `internal/*` packages should remain independent of each other. The only intentional cross-dependency today is `copier` → `db` (the safe-copy protocol records history inline).

---

## Branching: gitflow

This repo follows the standard [gitflow](https://nvie.com/posts/a-successful-git-branching-model/) model.

| Branch | Forks from | Merges into | Purpose |
|---|---|---|---|
| `main` | — | — | Production. Tagged commits only, never edited directly. |
| `develop` | `main` (initial) | `main` via `release/*` | Integration. Default working branch. |
| `feature/<name>` | `develop` | `develop` | New features, refactors, non-urgent fixes. |
| `release/<version>` | `develop` | `main` + back into `develop` | Release stabilization. Bump version, finalize changelog. |
| `hotfix/<version>` | `main` | `main` + back into `develop` | Emergency production fixes only. |

### Workflows

**Feature**

```bash
git checkout develop && git pull
git checkout -b feature/exif-date-extraction
# ... commits ...
git push -u origin feature/exif-date-extraction
# Open PR: feature/exif-date-extraction → develop
```

**Release**

```bash
git checkout develop && git pull
git checkout -b release/0.1.0
# bump version in cmd/ingest/main.go, update CHANGELOG.md, last-mile fixes
git push -u origin release/0.1.0
# PR 1: release/0.1.0 → main, merge with merge commit, tag v0.1.0
# PR 2: release/0.1.0 → develop (or merge main → develop), to back-port any release-stabilization commits
```

**Hotfix**

```bash
git checkout main && git pull
git checkout -b hotfix/0.1.1
# fix
git push -u origin hotfix/0.1.1
# PR 1: hotfix/0.1.1 → main, merge with merge commit, tag v0.1.1
# PR 2: hotfix/0.1.1 → develop, to keep develop ahead of main
```

### Hard rules

- **Never** commit directly to `main` or `develop`.
- **Never** force-push `main` or `develop`.
- All merges into `main` / `develop` go through a PR with at least one review.
- **Squash-merge** feature PRs into `develop` to keep history linear.
- **Merge-commit** release/hotfix PRs into `main` so the merge point is visible at the tag.
- Every commit on `main` is tagged `vX.Y.Z`.

---

## Commit messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `chore`, `build`, `ci`.

**Scopes** match package names: `scanner`, `device`, `period`, `template`, `copier`, `db`, `cli`. Use `repo` for cross-cutting changes.

**Examples**

```
feat(device): add Osmo Action 4 detection rule
fix(copier): clean tmp file when src stat fails before copy
refactor(template): extract segment renderer into helper
docs(readme): document --template syntax
```

The subject line is imperative mood, ≤72 characters, no trailing period.

---

## Code style

- Run `gofmt -s` before committing — CI will reject otherwise.
- `go vet ./...` must pass.
- Prefer the standard library. Current direct dependencies are intentionally minimal:
  - `github.com/spf13/cobra` — CLI framework
  - `github.com/cespare/xxhash/v2` — hashing
  - `modernc.org/sqlite` — pure-Go SQLite (no CGO, simpler cross-compile)
  Any new direct dependency needs justification in the PR description.
- **Comments**: write *why*, not *what*. Self-explanatory code is the goal; reach for comments only when an invariant, a workaround, or a non-obvious decision needs explaining.
- **Errors**: wrap with `fmt.Errorf("context: %w", err)` when callers might want `errors.Is/As`; otherwise plain `fmt.Errorf` is fine. Bubble errors up — do not log-and-return.
- **No globals** other than the cobra flag bindings in `cmd/ingest/main.go`. Pass dependencies explicitly.

---

## Adding a built-in device rule

Edit `internal/device/device.go`, append to `BuiltinRules`:

```go
{
    ID: "osmoaction4", Name: "DJI Osmo Action 4", Manufacturer: "DJI",
    VolumeLabels: []string{"OSMO"},
    Directories:  []string{"DCIM"},
    FilePatterns: []string{"DJI_*.MP4"},
},
```

Then verify:

```bash
go build ./... && ingest devices list
```

Confidence ordering (volume label → directory structure → file pattern → partial directory) is in `score()`. Don't add fields without first checking whether the existing match dimensions are sufficient.

YAML-driven external device rules (`devices.yaml`) are tracked under PRD FR-012.

---

## Modifying `copier.SafeCopy`

This is the most safety-sensitive code in the repo. Before changing it:

1. Re-read PRD §FR-005 in full.
2. Preserve these invariants:
   - The destination only becomes visible at its final path via `os.Rename(tmp, dst)`.
   - The hash recorded in the DB is the hash of the **source bytes**, verified to equal the hash of the **destination on disk**.
   - On any failure path, the temp file is removed.
   - Source file is read at most once during a copy (the in-flight stream feeds both the writer and the source-side hasher via `io.MultiWriter`).
3. When in doubt, write a test before touching the function.

---

## Testing

Tests are scaffolded for Phase 4 of the PRD; the repo does not yet have a test suite. When adding tests, prioritize:

1. `copier.SafeCopy` — happy path, hash mismatch, incremental skip, db record correctness, interrupted copy (kill mid-stream, verify no partial file at `dst`).
2. `template.Render` — segment omission with empty variable, escape handling, unknown variable error.
3. `device.Detect` — confidence ordering, multi-rule overlap, no-match case.

Run with:

```bash
go test ./...
go test -race ./...
```

---

## Releases

Maintainers only.

1. Open `release/<version>` from `develop`.
2. Bump `version` in `cmd/ingest/main.go`.
3. Update `CHANGELOG.md` (Keep a Changelog format).
4. PR → `main`, merge with merge commit.
5. Tag a signed annotated tag: `git tag -s v<version> -m "v<version>"`, then `git push --tags`.
6. Merge `main` back into `develop` (or merge the release branch into `develop`).
7. Build artifacts: `goreleaser` (planned) or `go build` per platform.

---

## Reporting bugs

Open an issue at <https://github.com/hktkzyx/ingest/issues> with:

- Platform + Go version (`go version`)
- Reproduction steps — a minimal source-directory layout helps a lot
- `--verbose` output and any failure messages

For data-corruption suspicions, attach the relevant SQLite row from `~/.local/share/ingest/ingest.db`:

```bash
sqlite3 ~/.local/share/ingest/ingest.db \
  "SELECT * FROM ingest_history WHERE target_path LIKE '%<file>%';"
```

---

## License

Same as the project (TBD — likely MIT or Apache-2.0).
