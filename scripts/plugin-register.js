#!/usr/bin/env node
// Helper: register/remove the oc-ops plugin entries in the global opencode.json.
//
// opencode v2 discovers plugins from the `plugins` array and still attempts to
// load entries in the legacy `plugin` array; the v1 implementation imports
// `@opencode-ai/plugin`, which is not installed for a v2-only setup, so leaving
// a v1 entry registered makes opencode report "Plugin failed to load".
//
// Default install therefore registers ONLY the v2 plugin (plugin-v2) and strips
// any stale v1 entry. Use target `v1` (legacy opencode 1.x) or `both` explicitly.
//
// Usage: node scripts/plugin-register.js [install|uninstall] [binaryPath] [v2|v1|both]
const fs = require("fs")
const path = require("path")

const mode = process.argv[2] ?? "install"
const binaryPath = process.argv[3] ?? "oc-ops"
const target = process.argv[4] ?? "v2"
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

function removeV1() {
  if (!Array.isArray(cfg.plugin)) return
  cfg.plugin = cfg.plugin.filter((e) => !isV1Entry(e))
  if (cfg.plugin.length === 0) delete cfg.plugin
}

function removeV2() {
  if (!Array.isArray(cfg.plugins)) return
  cfg.plugins = cfg.plugins.filter((e) => !isV2Entry(e))
  if (cfg.plugins.length === 0) delete cfg.plugins
}

function registerV1() {
  const v1 = Array.isArray(cfg.plugin) ? cfg.plugin : []
  const entry = v1.find(isV1Entry)
  if (entry) entry[1] = { binaryPath }
  else v1.push([v1Dir, { binaryPath }])
  cfg.plugin = v1
}

function registerV2() {
  const v2 = Array.isArray(cfg.plugins) ? cfg.plugins : []
  const entry = v2.find(isV2Entry)
  if (entry) entry.options = { ...(entry.options ?? {}), binaryPath }
  else v2.push({ package: v2Dir, options: { binaryPath } })
  cfg.plugins = v2
}

if (mode === "uninstall") {
  removeV1()
  removeV2()
  saveConfig(cfg)
  console.log(`removed oc-ops plugin entries from ${configJson}`)
  process.exit(0)
}

if (target === "v1" || target === "both") registerV1()
else removeV1()

if (target === "v2" || target === "both") registerV2()
else removeV2()

saveConfig(cfg)
const installed = [target === "v1" || target === "both" ? "v1 (plugin/)" : null, target === "v2" || target === "both" ? "v2 (plugin-v2/)" : null]
  .filter(Boolean)
  .join(" + ")
console.log(`registered oc-ops ${installed} in ${configJson} (binaryPath: ${binaryPath})`)