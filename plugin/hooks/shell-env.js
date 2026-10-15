// shell.env hook: inject the Android/NDK/JAVA toolchain and terminal overrides
// so command output is clean and builds stop tripping over missing env.
import { execFileSync } from "node:child_process"

// Resolve the Android env from the Go backend. Failures are non-fatal: we only
// inject what we actually resolved.
function resolveAndroidEnv(binaryPath, cwd) {
  try {
    const out = execFileSync(binaryPath, ["android_env"], { cwd, encoding: "utf8", timeout: 15000 })
    const resp = JSON.parse(out)
    const item = resp?.items?.[0]
    if (item && typeof item === "object") return item
  } catch {
    /* backend unavailable — skip env injection */
  }
  return {}
}

export function shellEnvHook(binaryPath, cwd) {
  return async (input, output) => {
    const env = output.env ?? {}
    const android = resolveAndroidEnv(binaryPath, input.cwd ?? cwd)
    if (android.ANDROID_HOME) env.ANDROID_HOME = android.ANDROID_HOME
    if (android.ANDROID_NDK_HOME) {
      env.ANDROID_NDK_HOME = android.ANDROID_NDK_HOME
      env.NDK_HOME = android.ANDROID_NDK_HOME
    }
    if (android.NDK_VERSION) env.NDK_VERSION = android.NDK_VERSION
    if (android.NDK_TOOLCHAIN) env.NDK_TOOLCHAIN = android.NDK_TOOLCHAIN
    if (android.JAVA_HOME) env.JAVA_HOME = android.JAVA_HOME
    // Terminal overrides: disable color/pager noise so output is token-clean.
    env.NO_COLOR = env.NO_COLOR ?? "1"
    env.PAGER = env.PAGER ?? "cat"
    env.GIT_PAGER = env.GIT_PAGER ?? "cat"
    env.TERM = env.TERM ?? "dumb"
    output.env = env
  }
}
