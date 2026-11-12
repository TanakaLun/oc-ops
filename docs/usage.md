# oc-ops Usage

## Prerequisites

- Go 1.27+ (`go` on PATH, or auto-detected at `/opt/go*/bin` / `/usr/local/go/bin`)
- `opencode` with plugin support — v2 (`@opencode/plugin`) or v1
  (`@opencode-ai/plugin` v1.18.15+)

## Build

```bash
cd oc-ops
make build          # -> bin/oc-ops
```

## Install

```bash
make install        # build + copy to $PREFIX/bin + register the opencode plugin
```

- **CLI**: `oc-ops` lands in `$PREFIX/bin`. On Termux `$PREFIX` defaults to
  `/data/data/com.termux/files/usr`; on other Unix-like systems the default is
  `/usr/local` (override with `make install PREFIX=~/.local` without root).
- **Plugin**: `plugin/` (v1) and `plugin-v2/` (v2) are registered in
  `~/.config/opencode/opencode.json` under the `plugin` / `plugins` arrays
  respectively, with the installed binary path as `binaryPath`.

## Direct CLI use

```bash
# summary-first grep (default outputMode)
oc-ops file grep '{"path":".","pattern":"foo","outputMode":"summary"}'

# flag form
oc-ops grep --path . --pattern 'foo' --output-mode count

# repo map
oc-ops map --path . --mode both --depth 6

# PTC: plan a no-effect batch, then resume it
oc-ops batch '{"action":"plan","operations":[{"op":"copy","path":"a.txt","to":"b.txt"}]}'
oc-ops batch '{"action":"resume","id":"b-..."}'

# PTC: store rows and page them
oc-ops result '{"action":"put","items":[{"n":1},{"n":2}]}'
oc-ops result '{"action":"page","id":"r-...","equals":{"n":1}}'
```

## opencode integration

The plugin registers `ops_*` tools. The model is told (via the routing section
and `AGENTS.md`) to prefer `ops_grep`/`ops_glob`/`ops_map` for searches and
exploration, `ops_inspect` for reading, `ops_replace` for batch edits, and
`ops_android_apk`/`ops_android_dex` for APK inspection.

## Environment normalization

With `shellEnv` on, every shell execution inherits `NO_COLOR=1`/`PAGER=cat`/
`GIT_PAGER=cat`/`TERM=dumb` so output stays clean. Toolchain paths (SDK/NDK/
JAVA_HOME) are your shell's responsibility — set them where your builds run,
not through oc-ops.

## Testing

```bash
make test           # go test ./...
make plugin-test    # node --test plugin/tools/test.mjs
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
