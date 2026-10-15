# Changelog

All notable changes to oc-ops are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com), and this
project adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added

- Go single-binary backend `oc-ops` with subcommands:
  - `file`: `grep` (summary-first), `glob`, `inspect` (text/hex), `replace`
    (dry-run by default, `apply:true` to write; read-only rejected)
  - `map`: repository structure map + language-aware symbol index
  - `bg` (`start`/`output`/`list`/`kill`): disk-backed background jobs with
    per-session ownership and bounded, spill-path output
  - `android`: `env` (NDK/JAVA/ANDROID_HOME resolution), `apk` (aapt2
    badging), `dex` (dex listing)
- opencode plugin (`@opencode-ai/plugin` v1) registering `ops_*` tools:
  - `ops_grep`, `ops_glob`, `ops_inspect`, `ops_replace`
  - `ops_map`
  - `ops_bg_start`, `ops_bg_output`, `ops_bg_list`, `ops_bg_kill` (opt-in)
  - `ops_android_env`, `ops_android_apk`, `ops_android_dex`
- `shell.env` hook injecting Android/NDK/JAVA toolchain and NO_COLOR/PAGER
  terminal overrides.
- `experimental.chat.system.transform` hook emitting a compact routing section
  (progressive disclosure — only enabled components are advertised).
- Strict config validation rejecting unknown keys/components.
- AGENTS.md for the oc-ops repo + per-repo-type template skeletons
  (Compose / reverse-engineering / Xposed).
- Go tests (file/repomap/bg) and plugin tests; Makefile targets
  (`build`, `test`, `plugin-test`, `vet`, `install`); GitHub Actions CI.