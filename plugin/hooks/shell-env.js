// shell.env hook: inject terminal overrides so command output is clean and
// token-lean. The Android/NDK/JAVA toolchain is deliberately NOT resolved or
// injected here — toolchain paths are the user's environment, not oc-ops's.
export function shellEnvHook() {
  return async (_input, output) => {
    const env = output.env ?? {}
    // Terminal overrides: disable color/pager noise so output is token-clean.
    env.NO_COLOR = env.NO_COLOR ?? "1"
    env.PAGER = env.PAGER ?? "cat"
    env.GIT_PAGER = env.GIT_PAGER ?? "cat"
    env.TERM = env.TERM ?? "dumb"
    output.env = env
  }
}