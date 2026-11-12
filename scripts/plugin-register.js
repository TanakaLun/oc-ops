#!/usr/bin/env node
// Helper: register/remove the oc-ops plugin entries in the global opencode.json.
// Registers BOTH the opencode v2 plugin (plugin-v2 -> `plugins` array) and the
// legacy v1 plugin (plugin -> `plugin` array), so either opencode generation
// loads oc-ops. The optional second argument is the installed binary path,
// written into the plugin options as binaryPath.
// Usage: node scripts/plugin-register.js [install|uninstall] [binaryPath]
const fs = require("fs")
const path = require("path")

const mode = process.argv[2] ?? "install"
const binaryPath = process.argv[3] ?? "oc-ops"
const repoRoot = path.resolve(__dirname, "..")
const v1Dir = path.resolve(repoRoot, "plugin")
const v2Dir = path.resolve(repoRoot, "plugin-v2")
const configJson = path.join(process.env.HOME, ".config", "opencode", "opencode.json")

function loadConfig() {
  if (!fs.existsSync(configJson)) return {}
  return JSON.parse(fs.readFileSync(configJson, "utf8"))
}

function saveConfig(cfg) {
  fs.mkdirSync(path.dirname(configJson), { recursive: true })
  fs.writeFileSync(configJson, JSON.stringify(cfg, null, 2) + "\n")
}

const isV1Entry = (e) => (Array.isArray(e) ? e[0] : e) === v1Dir
const isV2Entry = (e) => (typeof e === "string" ? e : e?.package) === v2Dir

const cfg = loadConfig()

if (mode === "uninstall") {
  if (Array.isArray(cfg.plugin)) cfg.plugin = cfg.plugin.filter((e) => !isV1Entry(e))
  if (Array.isArray(cfg.plugins)) cfg.plugins = cfg.plugins.filter((e) => !isV2Entry(e))
  saveConfig(cfg)
  console.log(`removed oc-ops plugin entries from ${configJson}`)
  return
}

// v1 registration: legacy `plugin` array ([dir, options] tuples).
const v1 = Array.isArray(cfg.plugin) ? cfg.plugin : []
const v1Entry = v1.find(isV1Entry)
if (v1Entry) v1Entry[1] = { binaryPath }
else v1.push([v1Dir, { binaryPath }])
cfg.plugin = v1

// v2 registration: `plugins` array (string or { package, options }).
const v2 = Array.isArray(cfg.plugins) ? cfg.plugins : []
const v2Entry = v2.find(isV2Entry)
if (v2Entry) v2Entry.options = { ...(v2Entry.options ?? {}), binaryPath }
else v2.push({ package: v2Dir, options: { binaryPath } })
cfg.plugins = v2

saveConfig(cfg)
console.log(`registered oc-ops v1 (plugin/) and v2 (plugin-v2/) in ${configJson} (binaryPath: ${binaryPath})`)