// Android tools: ops_android_env, ops_android_apk, ops_android_dex.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

const s = tool.schema

export function androidTools(binaryPath) {
  return {
    ops_android_env: tool({
      description:
        "Resolve and report the Android/NDK/JAVA toolchain environment. Use before Android builds to confirm ANDROID_HOME, NDK, and JAVA_HOME are set. The shell.env hook injects these automatically; this tool surfaces them for inspection.",
      args: {},
      execute: async (_args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "android_env", {}, { cwd: ctx.directory })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops android_env error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_android_apk: tool({
      description:
        "Inspect an APK's metadata (package name, version, sdk versions, permissions) via aapt2 badging. Use for quick APK identification before a full jadx decompile.",
      args: {
        apk: s.string().describe("Path to the APK"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "android_apk", args, { cwd: ctx.directory })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops android_apk error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_android_dex: tool({
      description:
        "List dex entries in an APK (compact). Use to identify dex structure; for full class/method analysis use jadx or dex2jar.",
      args: {
        apk: s.string().describe("Path to the APK"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "android_dex", args, { cwd: ctx.directory })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops android_dex error] ${String(err?.message ?? err)}`
        }
      },
    }),
  }
}
