# oc-ops Architecture

## Purpose

oc-ops is a token-efficiency and capability layer for **opencode** on Termux/Android.
It is inspired by `dsh-ops` (the DeepSeek Harness ops plugin) but adapted to the
opencode plugin API and this machine's toolchain.

The problem it solves is **not** "opencode lacks tools" — opencode already ships
`read`/`write`/`edit`/`glob`/`grep`/`bash`. The problem is **token waste, error
loops, and attention drift**:

- Host `grep`/`glob` return unbounded, unshaped output (whole files, every match).
- Model thrashes between tool calls trying to re-run a mistyped argument.
- Exploring a large unfamiliar repo costs dozens of `glob`+`read` round trips.
- Long Gradle/NDK builds occupy a foreground call until timeout.

## Design principles

1. **Compact, bounded, summary-first output.** Every read/search returns the
   minimum the model needs (`count`/`summary`/`files_with_matches` modes, hard
   limits, paging markers). Never dump a whole file unless asked.
2. **Progressive disclosure.** Only tools that are actually available are
   advertised, and a single routing section tells the model *which* tool for
   *which* task. Absent tools are never mentioned.
3. **Strict argument validation.** Undeclared keys and bad values fail fast with
   an actionable message, so the model corrects the call instead of silently
   running nothing and looping.
4. **Native acceleration without a Rust toolchain.** Go compiles to a single
   aarch64 ELF on this machine. No cross-compilation, no Rust dependency.
5. **No hidden global state.** The backend is a pure CLI: JSON in, JSON out.
   The plugin owns all session/policy concerns.
6. **Environment normalization.** `shell.env` disables color/pager noise so
   command output is clean and token-lean. Toolchain/env paths (SDK, NDK,
   JAVA_HOME) are the user's shell responsibility — oc-ops never resolves or
   injects them.

## Components

| Component | Backend | opencode surface | Default |
| --- | --- | --- | --- |
| `file` | Go `file` | `ops_grep`, `ops_glob`, `ops_inspect`, `ops_replace` | on |
| `map` | Go `repomap` | `ops_map` | on |
| `background` | Go `bg` | `ops_bg_start`, `ops_bg_output`, `ops_bg_list`, `ops_bg_kill` | off |
| `android` | Go `android` | `ops_android_apk`, `ops_android_dex` | on |
| `ptc` | Go `batch` + `result` | `ops_batch`, `ops_result` | on |
| `shell` | plugin hook | `shell.env` terminal overrides | on |
| `route` | plugin | `experimental.chat.system.transform` routing | on |

## Two layers

```
┌──────────────────────────────────────────────────────────────┐
│  opencode plugin (JS, @opencode/plugin v2 setup() API;        │
│  legacy plugin/ uses @opencode-ai/plugin v1 tool() API)       │
│  index.js · tools/* · hooks/shell.env · hooks/system.route    │
│  Registers ops_* tools, injects terminal overrides + routing. │
└──────────────────────────────┬───────────────────────────────┘
                               │ exec (CLI subprocess, JSON)
┌──────────────────────────────▼───────────────────────────────┐
│  oc-ops (Go single binary, $PREFIX/bin)                      │
│  cmd/oc-ops/main.go → internal/{contract,file,map,bg,android}│
│  JSON contract in, compact JSON/text out. No session state.  │
└──────────────────────────────────────────────────────────────┘
```

### CLI contract

The Go binary is a **pure function** of `(subcommand, args JSON, stdin, cwd,
env)`. It reads a request and writes one response. It never holds a database,
never daemonizes (except `bg start`), and never mutates the caller's env.

Two invocation shapes:

1. **Flag form** (Termux CLI): `oc-ops grep --path . --pattern 'Foo' --output-mode count`
2. **JSON form** (plugin): `oc-ops file grep '{"path":".","pattern":"Foo",...}'`

The plugin always uses the JSON form so the argument contract is identical to
the tool schema and the backend stays a pure mapper.

## Output shaping (the token win)

Every read/search response carries a stable envelope:

```json
{
  "status": "completed" | "rejected" | "error",
  "phase": "observe" | "verify",
  "complete": true | false,
  "truncated": false,
  "matched": 42,
  "omitted": 0,
  "total": 42,
  "nextOffset": 42,
  "items": [ ... ],
  "error": null
}
```

Rules:

- `content` mode emits at most `limit` lines, each `path:line:text` with a
  bounded per-line truncation.
- `summary`/`count`/`files_with_matches` never emit line bodies — only counts
  or paths. These are the default modes for large trees.
- `inspect` returns numbered lines or hex, bounded by `limit` (max 10 000).
- `map` returns one line per top-relevant symbol/file, sorted by importance.

## Security posture

- **Never follows symlinks.** `glob`/`grep`/`map` traverse via `os.Lstat` and
  skip symlinks, junctions, hidden files, `.git`, `node_modules`, `target`,
  `.venv`.
- **`replace` is read-only-by-default** and only writes when `--apply` is set,
  with a content-revision check and atomic rename.
- **No shell interpolation.** All paths/patterns pass as argv, never through a
  shell. `rg`-style pattern strings are escaped appropriately.
- **`bg`** runs commands with the caller's cwd and scrubbed env; job output is
  bounded and spill-path reported on truncation.

## PTC: programmatic tool calling

A pure-tool-calling capability modelled on dsh-ops's PTC component: the agent
drives structured, preflighted, auditable bulk operations instead of free-form
shell loops.

- **`ops_batch`** — `plan → run → resume` lifecycle for copy/move/remove.
  `plan` performs a full no-effect preflight (destination-exists, protected,
  overlapping and dependent path checks, source snapshot) and stores a durable
  audit record keyed by `id`. `resume` re-checks each source revision (CAS)
  before acting, then writes per-item outcomes. Read-only sessions accept
  `plan` but reject `run`/`resume`.
- **`ops_result`** — bounded, session-owned result references: `put` object
  rows, `page` with top-level `equals`/`fields` filters and quantity/byte
  budgets, `release`. 24h TTL. References grant no filesystem authority.
  `ops_batch` consumes stored operation rows via `operationsRef`.

Both persist under the shared disk store (`<cache>/oc-ops/{batch,result}`) so a
plan survives separate CLI invocations; each record is scoped to its owner
session.

## Environment normalization (`shell`)

`shell.env` injects terminal overrides:

- `NO_COLOR=1`, `PAGER=cat`, `GIT_PAGER=cat`, `TERM=dumb`

Toolchain paths (SDK, NDK, JAVA_HOME) are deliberately not resolved or injected
by oc-ops — they belong to the user's shell/environment.

## Why Go over Rust/C

- Go compiles to a single static-ish binary for any host (verified on
  `linux/amd64` WSL2 and `android/arm64` Termux).
- Single binary, no toolchain vendoring, no cargo registry.
- `regexp`, `filepath.WalkDir`, `io`, `os/exec` are all stdlib — zero deps.
- Memory-safe by default; no unsafe code required for this surface.

## Extending

Add a subcommand under `internal/<name>` that implements
`contract.Handler` (a function `(Request) -> Response`), register it in
`cmd/oc-ops/main.go`, and expose it from a plugin module under
`plugin/tools/<name>.js`. Update the routing section and component table.
