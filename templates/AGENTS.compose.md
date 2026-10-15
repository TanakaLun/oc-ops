# AGENTS.md — Jetpack Compose / Android app repo

Copy this file to a Compose/Kotlin Android project root and adapt the paths.
This is a **template**: the oc-ops global AGENTS.md stays generic; this file
carries the repo-specific rules.

## Work here

- **Language**: Kotlin, Jetpack Compose. Follow the existing package/module layout.
- **Build**: `./gradlew` (use `sh gradlew` on Termux). For native-code builds run
  the NDK toolchain prefix first, per the machine AGENTS.md — **do not build
  unless the user confirms.**
- **Where to run**: from the repo root; module paths are relative to it.

## Tool routing

- Explore the module tree first → `ops_map` (mode=both).
- Find code → `ops_grep` (outputMode=summary/count to stay token-lean).
- Read a file → `ops_inspect` (offset/limit for continuation).
- Batch edits → `ops_replace` (dry-run first, then `apply:true`).
- Confirm Android env before builds → `ops_android_env` (the shell.env hook
  sets ANDROID_HOME/NDK/JAVA automatically).

## Compose conventions to honor

- Prefer `remember`/`rememberSaveable` for state; hoist state to a
  `ViewModel`/holder when shared.
- Use `Modifier` chaining; place root layout via caller-provided modifiers.
- Mark composables as needed; avoid recomposition churn (stable params).
- Follow the module's own `State`/`Effect` naming.

## Don't

- Don't run `gradlew` builds without confirmation.
- Don't write to `$ANDROID_HOME` or modify SDK/NDK.
- Don't dump whole files when a bounded `ops_inspect`/`ops_grep` suffices.
