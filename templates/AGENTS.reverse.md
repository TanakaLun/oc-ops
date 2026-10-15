# AGENTS.md — Android reverse-engineering workspace

Copy this file to `~/work/逆向/` (or your RE workspace) and adapt. This is a
**template**: the global AGENTS.md stays generic; this file carries the
workspace-specific reversing rules.

## Work here

- **All reversing commands run in this directory.** Output goes to
  `output/`, scripts to `scripts/`, results to `analysis/`.
- APKs go in the workspace root (`base.apk`, `split_config.arm64_v8a.apk`, ...).

## Tool routing

- Identify an APK quickly → `ops_android_apk` (aapt2 badging).
- List dex structure → `ops_android_dex`; for full class/method analysis use
  `jadx`/`dex2jar` in a shell.
- Explore the decompiled tree → `ops_map` (mode=both) to orient.
- Search decompiled sources → `ops_grep` (summary-first).
- The reverse-engineering skill (`android-reverse-engineering`) provides the
  apktool/jadx/frida workflow — prefer it over ad-hoc commands.

## Available tooling

apktool, jadx, frida, frida-python, rizin, dex2jar, blutter, radare2, reflutter.

## Workflow

1. `ops_android_apk` to identify package/version/permissions.
2. Decompile with apktool/jadx as needed.
3. `ops_map` + `ops_grep` to navigate the decompiled code.
4. Audit for: hardcoded keys, SSL pinning, root/anti-debug detection, privacy
   leaks. Record findings in `analysis/`.

## Don't

- Don't run commands outside this workspace.
- Don't modify the original APK; keep derived artifacts in `output/`.
- Don't dump huge decompiled output into the model — use `ops_grep` summary.
