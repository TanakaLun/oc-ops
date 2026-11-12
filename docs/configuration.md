# oc-ops Configuration

The plugin reads a single `oc-ops` block from `opencode.json`. All keys are
optional; the defaults are safe.

## Example

The plugin is registered via the `plugins` array (v2) or the legacy `plugin`
array (v1). Settings live in the plugin `options`:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "plugins": [
    {
      "package": "/path/to/oc-ops/plugin-v2",
      "options": {
        "binaryPath": "oc-ops",
        "components": {
          "file": true,
          "map": true,
          "background": false,
          "android": true,
          "ptc": true,
          "shell": true,
          "route": true
        },
        "shellEnv": true,
        "routeSection": true
      }
    }
  ]
}
```

`make install` / `node scripts/plugin-register.js install <binaryPath>` writes
this entry (with your installed binary path) automatically. It registers the v2
plugin only and removes any stale legacy `plugin` entry — opencode v2 still
tries to load `plugin` entries, and the v1 implementation fails without
`@opencode-ai/plugin`. For opencode 1.x, register the legacy entry explicitly
with `node scripts/plugin-register.js install <binaryPath> v1` (or `make
plugin-install-v1`).

## Key reference

| Key | Type | Default | Belongs to | Notes |
| --- | --- | --- | --- | --- |
| `binaryPath` | string | `"oc-ops"` | all | Go binary; an absolute path or a name resolved on PATH |
| `required` (v1) | bool | `false` | all | Fail plugin activation if the binary is unusable |
| `toolCallTimeoutMs` (v1) | int | `300000` | all | Per-call deadline |
| `components.file` | bool | `true` | file | Enable the four `file` tools |
| `components.map` | bool | `true` | map | Enable `ops_map` |
| `components.background` | bool | `false` | background | Enable background job tools |
| `components.android` | bool | `true` | android | Enable android tools |
| `components.ptc` | bool | `true` | ptc | Enable `ops_batch` / `ops_result` |
| `components.shell` | bool | `true` | shell | Enable shell terminal overrides |
| `components.route` | bool | `true` | route | Enable the routing prompt section |
| `shellEnv` | bool | `true` | shell | Inject terminal overrides (NO_COLOR/PAGER/TERM) |
| `routeSection` | bool | `true` | route | Emit the routing section |
| `maxOutputLines` (v1) | int | `1000` | file/map | Hard cap on emitted lines |
| `maxFileBytes` (v1) | int | `33554432` | file | Per-file read budget |
| `extraGuidance` (v1) | string | `""` | route | Extra text appended to the routing section |

`(v1)` marks keys accepted only by the legacy `plugin/` implementation
(`@opencode-ai/plugin` v1). The v2 `plugin-v2/` implementation honors
`binaryPath`, `components.*`, `shellEnv`, and `routeSection`.

## Validation

Unknown keys are rejected with a clear error naming the key — the plugin never
silently ignores a config value (mirrors `dsh-ops`). Component booleans are
coerced to booleans; `toolCallTimeoutMs` must be a positive finite number.

## Component semantics

- **file/map/android/shell/route** default on; **background** off.
- `components.route` off hides the routing section but keeps the tools.
- `shellEnv` off disables only the `shell.env` hook; the tools remain.
- `required: true` makes a missing/unusable binary fatal at startup; otherwise
  the plugin logs a warning and disables the affected component.

## Interaction with the global AGENTS.md

The routing section emitted by the plugin and the routing guidance in
`AGENTS.md` are complementary: `AGENTS.md` sets the *default* preferences; the
plugin's section reflects *what is actually installed* for this session. When
both are present, the plugin's section is the authoritative "which tool for
which task" for the running session.
