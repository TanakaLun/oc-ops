// oc-ops opencode plugin entry.
// Registers ops_* tools backed by the oc-ops Go binary, injects the Android
// toolchain env, and adds a token-routing section to the system prompt.
import { resolveConfig } from "./config.js"
import { fileTools } from "./tools/file.js"
import { mapTool } from "./tools/map.js"
import { backgroundTools } from "./tools/background.js"
import { androidTools } from "./tools/android.js"
import { shellEnvHook } from "./hooks/shell-env.js"
import { routeSectionHook } from "./hooks/system-route.js"

export const OcOpsPlugin = async (input, options) => {
  const log = (level, message) => {
    input.client.app
      .log({ body: { service: "oc-ops", level, message } })
      .catch(() => {})
  }

  // Load config: plugin options first, then the `oc-ops` block from
  // opencode.json, with defaults for anything else.
  let config
  try {
    const raw = options ?? {}
    config = resolveConfig(raw)
  } catch (err) {
    log("warn", `oc-ops disabled: ${err.message}`)
    return {}
  }

  const binaryPath = config.binaryPath
  const tools = {}
  if (config.components.file) Object.assign(tools, fileTools(binaryPath))
  if (config.components.map) Object.assign(tools, mapTool(binaryPath))
  if (config.components.background) Object.assign(tools, backgroundTools(binaryPath))
  if (config.components.android) Object.assign(tools, androidTools(binaryPath))

  const hooks = {}
  if (Object.keys(tools).length) hooks.tool = tools
  if (config.shellEnv && config.components.shell) {
    hooks["shell.env"] = shellEnvHook(binaryPath, input.directory)
  }
  if (config.routeSection && config.components.route) {
    hooks["experimental.chat.system.transform"] = routeSectionHook(
      config.components,
      config.extraGuidance,
    )
  }

  log("info", `oc-ops ready (${Object.keys(tools).length} tools, components: ${Object.entries(config.components).filter(([, v]) => v).map(([k]) => k).join("+")})`)
  return hooks
}

export default OcOpsPlugin