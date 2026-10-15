# oc-ops Usage

## Prerequisites

- Termux with `go` installed (verified: `go1.27.1 android/arm64`)
- `opencode` with `@opencode-ai/plugin` available (v1.18.15 installed)

## Build

```bash
cd oc-ops
make build          # -> bin/oc-ops
```

The Go binary compiles natively for `aarch64` (verified working ELF).

## Install

```bash
make install        # build + copy to $PREFIX/bin, link plugin to ~/.config/opencode/plugins
```

- **CLI**: `oc-ops` lands in `$PREFIX/bin` so it is on PATH and callable
  directly in a Termux shell.
- **Plugin**: `plugin/` is linked into `~/.config/opencode/plugins/` so opencode
  loads it at startup.

## Direct CLI use

```bash
# summary-first grep (default outputMode)
oc-ops file grep '{"path":".","pattern":"foo","outputMode":"summary"}'

# flag form
oc-ops grep --path . --pattern 'foo' --output-mode count

# repo map
oc-ops map --path . --mode both --depth 6

# android env (for inspection / sourcing)
oc-ops android env
```

## opencode integration

The plugin registers `ops_*` tools. The model is told (via the routing section
and `AGENTS.md`) to prefer `ops_grep`/`ops_glob`/`ops_map` for searches and
exploration, `ops_inspect` for reading, `ops_replace` for batch edits, and
`ops_android_env`/`ops_android_apk` for Android work.

## Environment injection

With `shellEnv` on, every shell execution inherits `ANDROID_HOME`,
`ANDROID_NDK_HOME`, `JAVA_HOME`, and `NO_COLOR=1`/`PAGER=cat`/`GIT_PAGER=cat`/
`TERM=dumb`. This removes the recurring NDK/JAVA build loops.

## Testing

```bash
make test           # go test ./...
cd plugin && bun test
```

## Disabling

Edit `opencode.json` and set `components.*` to `false`, or set the whole
`oc-ops` block to disable. Restart opencode.

## Troubleshooting

- **"oc-ops: binary not found"** — `make install` again, or set
  `binaryPath` to the absolute path of the compiled binary.
- **`required: true` startup failure** — the binary could not be resolved or
  spawned; check `make build` succeeded and the path is correct.
- **Empty `ops_grep`** — confirm `outputMode`; `summary` returns counts only.
  Use `outputMode: content` to see lines.
- **`ops_replace` did nothing** — `apply` is false by default; pass `apply: true`
  to write, and use `dryRun` first to observe.
