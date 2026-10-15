# AGENTS.md — oc-ops

This AGENTS.md governs development **on the oc-ops repo itself**. oc-ops is a
token-efficiency + capability layer for opencode on Termux/Android: a Go CLI
backend plus an opencode plugin.

## Project layout

```
oc-ops/
├── cmd/oc-ops/         Go main entry + subcommand dispatch
├── internal/
│   ├── contract/       JSON request/response envelope + walk helpers
│   ├── file/           grep/glob/inspect/replace
│   ├── repomap/        repo map + symbol index
│   ├── bg/             background jobs (disk-backed)
│   ├── batch/          PTC plan/run/resume copy/move/remove
│   ├── result/         PTC bounded result references
│   ├── store/          shared disk-backed JSON store
│   └── android/        NDK/JAVA env + apk/dex
├── plugin/             opencode plugin (JS, @opencode-ai/plugin v1)
│   ├── index.js        entry
│   ├── config.js       config resolution + validation
│   ├── cli.js          Go binary invocation + rendering
│   ├── tools/          one module per tool group
│   └── hooks/          shell.env + system routing
├── templates/          per-repo-type AGENTS.md skeletons
└── docs/               architecture / components / configuration / usage
```

## Build & test

```bash
make build      # -> bin/oc-ops
make test       # go test ./...
make vet        # go vet ./...
make install    # build + install to $PREFIX/bin + link plugin
```

## Conventions

- **Single Go binary, subcommand dispatch.** Add a capability under
  `internal/<name>` implementing a handler, register it in
  `cmd/oc-ops/main.go`.
- **Backend is a pure function.** It reads `(subcommand, args JSON, cwd, env)`
  and writes one JSON response. No session state, no daemon (except bg jobs),
  no shell interpolation.
- **Envelope is mandatory.** Every handler returns the `contract.Response`
  envelope with `status/phase/matched/total/nextOffset/items/error`. Never
  return a bare string.
- **Summary-first, bounded output.** Default modes are `summary`/`count`.
  `content` is opt-in and always bounded by `limit`. Cap line bodies.
- **Strict argument validation.** Reject unknown keys with a clear message
  naming the accepted set — this breaks error loops.
- **No symlink traversal.** `glob`/`grep`/`repomap` use `Walk` which skips
  symlinks, hidden files, `.git`, `node_modules`, `target`, `.venv`, `.gradle`.
- **`replace` is dry-run by default.** It writes only with `apply:true`, and is
  rejected in read-only mode.
- **Env injection is non-fatal.** The `shell.env` hook injects what it resolved
  and silently skips what it did not.
- **No comments unless asked.** Code is self-documenting; module headers
  explain intent.

## Tool routing (what the model should prefer)

- Searches/exploration → `ops_grep` (summary-first) / `ops_glob` / `ops_map`
- Reading a file → `ops_inspect`
- Batch edits → `ops_replace` (dry-run first)
- Android builds → `ops_android_env` + the shell.env-injected env
- Long-running processes → `ops_bg_start`/`ops_bg_output`/`ops_bg_kill`
- Programmatic bulk copy/move/remove → `ops_batch` (`plan` → `resume`)
- Bounded result references → `ops_result` (`put`/`page`/`release`)

## PTC conventions

- `ops_batch` **plan** must be used before any mutation; **resume** (never
  `run` for a planned set) executes the exact plan by `id`. `plan` has no
  effects; `run`/`resume` are rejected in read-only sessions.
- Sources are snapshotted at plan time and re-checked (CAS) at resume time —
  never clobber a source that changed after preflight.
- No overwrite, no symlink traversal, no protected/overlapping/dependent paths.
- `ops_result` rows are observations, never authority; page them with
  `equals`/`fields` and quantity/byte budgets. `ops_batch` consumes stored
  operation rows via `operationsRef`.
- Records live in the shared store (`internal/store`) and are session-owned;
  validate ids with `store.ValidateID` and never build paths from raw args.

## Upstream-compat note

The plugin uses `@opencode-ai/plugin` **v1** (`tool()` helper, `Hooks` object),
not the v2 `define` API. The installed version is `1.18.15`. When a hook or
tool shape is added, confirm it exists in the v1 `Hooks` interface.
