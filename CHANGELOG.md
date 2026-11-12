# Changelog

All notable changes to oc-ops are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com), and this
project adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

### Changed

- **opencode v2 plugin (`plugin-v2/`, zero-dependency):** `binaryPath` from
  `ctx.options` (default `oc-ops` on PATH), per-component toggles, and a routing
  section built from the enabled components.
- **Platform-aware `make install`:** non-Termux prefix defaults to `/usr/local`
  (override with `make install PREFIX=~/.local`); Go toolchain auto-detected
  from PATH, `/opt/go*/bin`, `/usr/local/go/bin`.
- **Plugin registration script:** manages both the legacy v1 (`plugin`) and v2
  (`plugins`) entries and writes the installed binary path into plugin options.

### Removed

- **Android/NDK/JAVA toolchain path resolution:** `ops_android_env`, the
  ANDROID_HOME/ANDROID_NDK_HOME/JAVA_HOME probing in `internal/android`, and
  toolchain injection from the `shell.env` hook. Toolchain paths are the user's
  shell responsibility; only APK inspection (`ops_android_apk`,
  `ops_android_dex`) remains in the `android` component.

### Fixed

- Namespace CLI form `oc-ops file grep '{"path":...}'` no longer leaks the
  `tool` key into handler args (it was rejected by strict file validation).
- Test-suite race: `go test ./...` packages deleted the shared store root
  concurrently; each test now clears only its own collection directory.

### Added

- Go single-binary backend `oc-ops` with subcommands:
  - `file`: `grep` (summary-first), `glob`, `inspect` (text/hex), `replace`
    (dry-run by default, `apply:true` to write; read-only rejected)
  - `map`: repository structure map + language-aware symbol index
  - `bg` (`start`/`output`/`list`/`kill`): disk-backed background jobs with
    per-session ownership and bounded, spill-path output
  - `android`: `env` (NDK/JAVA/ANDROID_HOME resolution), `apk` (aapt2
    badging), `dex` (dex listing)
  - `batch`: PTC `plan`/`run`/`resume` for copy/move/remove — no-effect
    preflight, durable on-disk audit, CAS re-checks, no overwrite/link
    traversal/protected paths, session ownership, read-only rejection
  - `result`: PTC bounded result references `put`/`page`/`release` — 24h TTL,
    top-level `equals`/`fields` filters, quantity/byte budgets, references
    are observations never authority
  - `store`: shared collision-safe disk-backed JSON store (atomic save/load/
    delete/list)
- opencode plugin (`@opencode-ai/plugin` v1) registering `ops_*` tools:
  - `ops_grep`, `ops_glob`, `ops_inspect`, `ops_replace`
  - `ops_map`
  - `ops_bg_start`, `ops_bg_output`, `ops_bg_list`, `ops_bg_kill` (opt-in)
  - `ops_android_env`, `ops_android_apk`, `ops_android_dex`
  - `ops_batch`, `ops_result` (PTC component, on by default)
- `shell.env` hook injecting Android/NDK/JAVA toolchain and NO_COLOR/PAGER
  terminal overrides.
- `experimental.chat.system.transform` hook emitting a compact routing section
  (progressive disclosure — only enabled components are advertised).
- Strict config validation rejecting unknown keys/components.
- AGENTS.md for the oc-ops repo + per-repo-type template skeletons
  (Compose / reverse-engineering / Xposed).
- Go tests (file/repomap/bg/batch/result/store) and plugin tests; Makefile
  targets (`build`, `test`, `plugin-test`, `vet`, `install`); GitHub Actions CI.