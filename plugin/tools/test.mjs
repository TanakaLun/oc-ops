// Minimal plugin tests using node's built-in test runner (no deps).
// Run: node --test plugin/tools/*.test.mjs
import { test } from "node:test"
import assert from "node:assert"
import { DEFAULT_CONFIG, resolveConfig, ConfigError } from "../config.js"
import { renderRouting } from "../hooks/system-route.js"
import { renderResponse, tryParse } from "../cli.js"

test("resolveConfig defaults", () => {
  const c = resolveConfig({})
  assert.equal(c.binaryPath, "oc-ops")
  assert.equal(c.components.file, true)
  assert.equal(c.components.background, false)
  assert.equal(c.components.ptc, true)
  assert.equal(c.shellEnv, true)
})

test("resolveConfig rejects unknown keys", () => {
  assert.throws(() => resolveConfig({ nonsense: 1 }), ConfigError)
  assert.throws(() => resolveConfig({ components: { nonsense: true } }), ConfigError)
})

test("resolveConfig validates component types", () => {
  assert.throws(() => resolveConfig({ components: { file: "yes" } }), ConfigError)
  assert.throws(() => resolveConfig({ toolCallTimeoutMs: -5 }), ConfigError)
})

test("renderRouting reflects enabled components", () => {
  const all = renderRouting({ file: true, map: true, background: true, android: true, ptc: true })
  assert.ok(all.includes("ops_grep"))
  assert.ok(all.includes("ops_map"))
  assert.ok(all.includes("ops_bg_start"))
  assert.ok(all.includes("ops_android_env"))
  assert.ok(all.includes("ops_batch"))
  assert.ok(all.includes("ops_result"))
  const none = renderRouting({ file: false, map: false, background: false, android: false, ptc: false })
  assert.equal(none, "")
})

test("renderResponse summary format", () => {
  const resp = {
    status: "completed", phase: "observe", matched: 3, total: 3, complete: true,
    items: [{ path: "a.txt", line: 1, text: "hi" }],
  }
  const txt = renderResponse(resp)
  assert.ok(txt.includes("matched: 3"))
  assert.ok(txt.includes("a.txt"))
})

test("renderResponse error format", () => {
  const txt = renderResponse({ status: "error", error: { code: "INVALID_ARGUMENT", message: "bad", nextAction: "fix" } })
  assert.ok(txt.includes("INVALID_ARGUMENT"))
  assert.ok(txt.includes("fix"))
})

test("tryParse handles non-JSON", () => {
  assert.equal(tryParse("not json"), null)
  assert.equal(tryParse(""), null)
  assert.deepEqual(tryParse('{"a":1}'), { a: 1 })
})