# AGENTS.md — Xposed / LSPosed module repo

Copy this file to an Xposed module project root and adapt. This is a
**template**: the global AGENTS.md stays generic; this file carries the
module-specific rules.

## Work here

- **Language**: Kotlin/Java. Use the modern Xposed API (libxposed) with the
  chained `Hooker`/`Chain` model — not the legacy `XposedHelpers`/`XposedBridge`.
- **Hooking**: use `hook(Executable)` + `HookBuilder`, `replaceHook`,
  `deoptimize`, and `META-INF/xposed/java_init.list` for entry.
- **Reflection**: prefer KavaRef (`resolve()` DSL) for accessing obfuscated
  members; use DexKit for finding obfuscated classes/methods/fields by
  string/opcode/call-relation.

## Tool routing

- Find the target members → `ops_grep` (search strings/classes) + DexKit skills.
- Explore module structure → `ops_map`.
- Batch edits across the module → `ops_replace` (dry-run first).
- Confirm Android env before build → `ops_android_env`.

## Module conventions

- `module.prop` declares id/version/description.
- `scope.list` (if used) lists target packages.
- Dependencies: `api` is compileOnly for the Xposed API; `service` is
  implementation for remote preferences/provider.

## Don't

- Don't use legacy `findAndHookMethod`/`XposedHelpers`/`de.robv.android.xposed.*`.
- Don't call hooks from a global context when a scoped module is intended.
- Don't build without confirmation.
