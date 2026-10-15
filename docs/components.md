# oc-ops Component Reference

Each component maps to a set of opencode tools and a Go backend subcommand.
Tools use the `ops_` prefix, matching the `dsh-ops` convention.

## Component overview

| Component | Backend subcommand | Published tools | Default |
| --- | --- | --- | --- |
| `file` | `file` | `ops_grep`, `ops_glob`, `ops_inspect`, `ops_replace` | on |
| `map` | `map` | `ops_map` | on |
| `background` | `bg` | `ops_bg_start`, `ops_bg_output`, `ops_bg_list`, `ops_bg_kill` | off |
| `android` | `android` | `ops_android_env`, `ops_android_apk`, `ops_android_dex` | on |
| `ptc` | `batch`, `result` | `ops_batch`, `ops_result` | on |
| `shell` | `android/env` + `shell.env` | (env injection only) | on |
| `route` | — | (system prompt routing) | on |

## Tool surface

### `file` — `ops_inspect`

Read bounded UTF-8 text lines or a hex view of a single file.

| Arg | Type | Notes |
| --- | --- | --- |
| `path` | string | Relative to session cwd; must be a file |
| `offset` | int ≥ 1 | 1-based start line |
| `limit` | int 1–10000 | Max lines |
| `view` | `text`\|`hex` | Default `text` |

Directory paths are rejected — use `ops_glob`/`ops_grep`.

### `file` — `ops_grep`

Rust-regex-style search under `path`, bounded and summary-first.

| Arg | Type | Notes |
| --- | --- | --- |
| `pattern` | string (required) | Go regexp |
| `path` | string | Search root (default cwd) |
| `glob` | string[] | Include filters; `!` excludes |
| `outputMode` | `content`\|`files_with_matches`\|`count`\|`summary` | Default `summary` |
| `caseInsensitive` | bool | |
| `lineNumbers` | bool | |
| `beforeContext`/`afterContext` | int ≤ 32 | |
| `offset`/`limit` | int | Pagination |

`summary` is the default: returns only `files`/`matches` counts. `count` returns
per-file counts. `files_with_matches` returns paths only. Only `content` emits
line bodies.

### `file` — `ops_glob`

Find files under `path` by explicit patterns.

| Arg | Type | Notes |
| --- | --- | --- |
| `path` | string | Search root |
| `pattern` | string[] (required) | Glob patterns; `!` excludes |
| `outputMode` | `paths`\|`details` | Default `paths` |
| `sort` | `path`\|`modified` | Default `path` |
| `offset`/`limit` | int | Pagination |

### `file` — `ops_replace`

Deterministic UTF-8 replacement. **Dry-run by default.**

| Arg | Type | Notes |
| --- | --- | --- |
| `path` | string | Root |
| `pattern` | string (required) | Go regexp |
| `replacement` | string (required) | |
| `glob` | string[] | Include filters |
| `maxReplacements` | int 1–100000 | Hard cap |
| `apply` | bool | **Writes when true**; otherwise observe only |
| `dryRun` | bool | Explicit observe |

`apply` is intentionally false by default — the model must confirm before
writing, mirroring dsh-ops's revision-checked atomic replacement.

### `map` — `ops_map`

Produce a compact structure map + symbol index of a repository.

| Arg | Type | Notes |
| --- | --- | --- |
| `path` | string | Repo root (default cwd) |
| `depth` | int 1–12 | Directory depth to show |
| `mode` | `tree`\|`symbols`\|`both` | Default `both` |
| `glob` | string[] | Include filters |
| `offset`/`limit` | int | Pagination |

`tree` emits one line per directory/file. `symbols` emits one line per
top-relevant symbol (function/class/interface/type) as `path:line:name` for
code files, using language-aware line heuristics. `both` emits a tree then a
symbol index. This is the single biggest token saving when exploring an
unfamiliar repo.

### `background` — `ops_bg_start`

Start a long-running command as a managed background job.

| Arg | Type | Notes |
| --- | --- | --- |
| `command` | string (required) | Bash command |
| `cwd` | string | Default session cwd |
| `label` | string | Human label |
| `timeoutMs` | int | Max lifetime |

Returns a `job_id`.

### `background` — `ops_bg_output` / `ops_bg_list` / `ops_bg_kill`

Read / list / terminate jobs started in this session. Only owned jobs are
visible. `output` supports `after_seq` for explicit continuation.

### `android` — `ops_android_env`

Print the resolved Android/NDK/JAVA environment as JSON, for the plugin's
`shell.env` hook and for manual `source`/inspection.

### `android` — `ops_android_apk`

Lightweight APK inspection (aapt2-based) — PACKAGE name, version, perms,
components — without a full jadx decompile.

### `android` — `ops_android_dex`

List class/method shapes in a dex/apk (uses dex2jar or a dex parser),
producing compact `path:line:signature` lines.

### `ptc` — `ops_batch`

Preflighted, durably-audited batch copy/move/remove. The PTC lifecycle:

| Arg | Type | Notes |
| --- | --- | --- |
| `action` | `plan`\|`run`\|`resume` (required) | `plan` preflights with no effect; `run` validates + executes; `resume` executes an existing plan by `id` |
| `operations` | array ≤ 512 | Each `{op: copy\|move\|remove, path, to?}` |
| `operationsRef` | string | ops_result id holding operation rows; replaces operations |
| `root` | string | Default session cwd |
| `protect` | array ≤ 128 | Paths never touched |
| `id` | string | Plan id to resume |

- `plan` returns `id` + item list; nothing is written.
- `run`/`resume` require write access; read-only sessions reject them.
- No overwrite (destination must not exist), no symlink traversal, no
  protected/overlapping/dependent paths.
- `resume` re-checks each source revision (CAS) before acting, so a source
  changed after preflight is never silently clobbered.
- Plans persist on disk and are session-owned.

### `ptc` — `ops_result`

Bounded, session-owned result references.

| Arg | Type | Notes |
| --- | --- | --- |
| `action` | `put`\|`page`\|`release` (required) | |
| `items` | array ≤ 10000 | Object rows for put |
| `id` | string | Reference id for page/release |
| `offset`/`limit` | int | Pagination; limit 1..512 |
| `maxBytes` | int | Page byte budget 512..65536 |
| `fields` | array ≤ 32 | Project only these top-level fields |
| `equals` | object | Filter: rows matching these top-level fields |

- `put` returns `id` + expiry (24h).
- `page` reads rows with filters/projection and quantity/byte budgets.
- `release` frees a reference.
- References are observations, never authority — paging grants no filesystem
  access. Used by `ops_batch` `operationsRef`.

## Env normalization (`shell`)

Injected via the plugin `shell.env` hook: `ANDROID_HOME`, `ANDROID_NDK_HOME`,
`JAVA_HOME`, plus `NO_COLOR=1`, `PAGER=cat`, `GIT_PAGER=cat`, `TERM=dumb`.

## Related docs

- [Architecture](architecture.md)
- [Configuration](configuration.md)
- [Usage](usage.md)
