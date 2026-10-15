# oc-ops

Token-efficiency and capability layer for **opencode** on Termux/Android.

Inspired by [dsh-ops](https://github.com/T-Auto/dsh-ops) (the DeepSeek Harness
ops plugin), oc-ops gives opencode a **Go-native backend** plus an **opencode
plugin** that reduce token waste, stop error loops, and keep the model focused.

> ops 是广义 operations（运行操作）的缩写：oc-ops 处于 opencode 和
> 系统/代码/文件操作之间的基础工具位置，不是运维平台。

## What it fixes

opencode already ships `read`/`write`/`edit`/`glob`/`grep`/`bash`. The problems
on this machine are **token waste, error loops, and attention drift**:

- Host grep/glob return unbounded, unshaped output.
- Model thrashes between tool calls to re-run a mistyped argument.
- Exploring a large unfamiliar repo costs dozens of glob+read round trips.
- Long Gradle/NDK builds occupy a foreground call until timeout.
- Android builds trip over unset `NDK`/`JAVA_HOME`/`ANDROID_HOME`.

## Components

| Component | Tools / surface | Default |
| --- | --- | --- |
| `file` | `ops_grep`, `ops_glob`, `ops_inspect`, `ops_replace` | on |
| `map` | `ops_map` (repo structure + symbol index) | on |
| `background` | `ops_bg_start`, `ops_bg_output`, `ops_bg_list`, `ops_bg_kill` | off |
| `android` | `ops_android_env`, `ops_android_apk`, `ops_android_dex` | on |
| `ptc` | `ops_batch` (plan/run/resume), `ops_result` (put/page/release) | on |
| `shell` | `shell.env` injection (NDK/JAVA/ANDROID_HOME, NO_COLOR) | on |
| `route` | system-prompt routing section (progressive disclosure) | on |

## Architecture

Two layers: a **single Go binary** (`oc-ops`, pure-function JSON in/out) and an
**opencode plugin** (`@opencode-ai/plugin` v1 `tool()` API) that drives it.

```
opencode plugin (JS)  --exec-->  oc-ops (Go, $PREFIX/bin)
ops_grep/glob/inspect/            grep/glob/inspect/replace/map/
replace/map/bg_*/ptc/android      bg_start/output/list/kill + batch/result
* + hooks                         + android env/apk/dex
```

## Install

```bash
make install    # build + install to $PREFIX/bin + link plugin to ~/.config/opencode/plugins
```

Prerequisites: Termux with `go` (verified `go1.27.1 android/arm64`) and
`opencode` with `@opencode-ai/plugin` available.

## Test

```bash
make test         # go test ./...
make plugin-test  # node --test plugin/tools/test.mjs
make vet          # go vet ./...
```

## Configure

See [docs/configuration.md](docs/configuration.md). The plugin reads an
`oc-ops` block from `opencode.json`; all keys optional.

## Docs

- [Architecture](docs/architecture.md)
- [Components](docs/components.md)
- [Configuration](docs/configuration.md)
- [Usage](docs/usage.md)
- AGENTS.md: copy `templates/` skeletons into the relevant repos

## License

MIT — see [LICENSE](LICENSE).