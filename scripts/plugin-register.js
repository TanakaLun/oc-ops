#!/usr/bin/env node
// Helper: register/remove the oc-ops plugin in the global opencode.json.
// Usage: node scripts/plugin-register.js [install|uninstall]
const fs = require("fs")
const path = require("path")

const mode = process.argv[2] ?? "install"
const configJson = path.join(process.env.HOME, ".config", "opencode", "opencode.json")
const pluginDir = path.resolve(__dirname, "..", "plugin")

const cfg = JSON.parse(fs.readFileSync(configJson, "utf8"))
const list = Array.isArray(cfg.plugin) ? cfg.plugin : []
const isEntry = (e) => (Array.isArray(e) ? e[0] : e) === pluginDir

if (mode === "uninstall") {
  if (list.some(isEntry)) {
    cfg.plugin = list.filter((e) => !isEntry(e))
    fs.writeFileSync(configJson, JSON.stringify(cfg, null, 2) + "\n")
  }
  console.log(`removed oc-ops plugin from ${configJson}`)
  return
}

if (list.some(isEntry)) {
  console.log(`oc-ops plugin already registered in ${configJson}`)
  return
}
cfg.plugin = [...list, [pluginDir, {}]]
fs.writeFileSync(configJson, JSON.stringify(cfg, null, 2) + "\n")
console.log(`registered oc-ops plugin in ${configJson}`)
