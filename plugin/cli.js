// CLI bridge: invoke the oc-ops Go binary and parse its JSON response.
// The backend is a pure function of (subcommand, args JSON, cwd, env).
import { execFile } from "node:child_process"

// Run one oc-ops capability. Returns the parsed Response envelope.
export function invokeCLI(binaryPath, tool, args, { cwd, timeoutMs = 300000, readOnly = false, owner = "" } = {}) {
  return new Promise((resolve, reject) => {
    const payload = {
      ...args,
      ...(owner ? { owner } : {}),
      ...(readOnly ? { read_only: true } : {}),
      ...(cwd ? { cwd } : {}),
    }
    execFile(
      binaryPath,
      [tool, JSON.stringify(payload)],
      { cwd, timeout: timeoutMs, maxBuffer: 8 * 1024 * 1024, env: process.env },
      (err, stdout, stderr) => {
        if (err) {
          // A non-zero exit with a parseable JSON response is a structured error.
          const parsed = tryParse(stdout)
          if (parsed) return reject(new Error(parsed.error?.message ?? `oc-ops ${tool} failed: ${stderr || err.message}`))
          return reject(new Error(`oc-ops ${tool} failed: ${err.message}${stderr ? `\n${stderr}` : ""}`))
        }
        const parsed = tryParse(stdout)
        if (!parsed) return reject(new Error(`oc-ops ${tool} returned invalid output`))
        resolve(parsed)
      },
    )
  })
}

// Try to parse a string as the oc-ops JSON envelope.
export function tryParse(text) {
  if (!text) return null
  try {
    const obj = JSON.parse(text)
    if (obj && typeof obj === "object") return obj
  } catch {
    /* not JSON */
  }
  return null
}

// Render the Response envelope into model-facing text.
// Summary-first: the model sees counts and a compact list, never a raw dump.
export function renderResponse(resp) {
  if (!resp) return "(no result)"
  if (resp.error) {
    return `[oc-ops ${resp.error.code}] ${resp.error.message}${resp.error.nextAction ? `\nNext: ${resp.error.nextAction}` : ""}`
  }
  const lines = []
  if (resp.phase) lines.push(`phase: ${resp.phase}`)
  const markers = []
  if (resp.matched !== undefined) markers.push(`matched: ${resp.matched}`)
  if (resp.changed) markers.push(`changed: ${resp.changed}`)
  if (resp.total !== undefined) markers.push(`total: ${resp.total}`)
  if (markers.length) lines.push(markers.join(" · "))
  const items = resp.items ?? []
  for (const item of items) {
    if (typeof item === "string") {
      lines.push(item)
    } else if (item && typeof item === "object") {
      lines.push(formatItem(item))
    }
  }
  if (resp.truncated) lines.push("(output truncated — use offset/limit to page)")
  if (resp.nextOffset !== undefined && !resp.complete) lines.push(`nextOffset: ${resp.nextOffset}`)
  return lines.join("\n") || "(no items)"
}

function formatItem(item) {
  // Prefer a compact single-line shape.
  const parts = []
  if (item.path !== undefined) parts.push(String(item.path))
  if (item.line !== undefined) parts.push(String(item.line))
  if (item.name !== undefined) parts.push(String(item.name))
  if (item.count !== undefined) parts.push(`count:${item.count}`)
  if (item.text !== undefined) parts.push(String(item.text))
  if (item.job_id !== undefined) parts.push(`job:${item.job_id}`)
  if (item.done !== undefined) parts.push(`done:${item.done}`)
  if (item.hex !== undefined) parts.push(item.hex)
  if (parts.length) return parts.join(": ")
  // Fallback: JSON with a byte cap.
  return JSON.stringify(item).slice(0, 2000)
}
