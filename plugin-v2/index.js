// oc-ops — OpenCode v2 plugin (zero dependency).
// Reuses the oc-ops Go backend (JSON in/out) and exposes ops_* tools plus
// shell-env injection and a system-prompt routing section, using the
// @opencode/plugin v2 API: default export { id, setup(ctx) }.
import { execFile } from "node:child_process"

const BIN = "/usr/local/bin/oc-ops"
const TIMEOUT = 300000
const MAXBUF = 8 * 1024 * 1024

function invoke(tool, args, cwd, owner) {
  return new Promise((resolve, reject) => {
    const payload = { ...(args ?? {}), ...(owner ? { owner } : {}), ...(cwd ? { cwd } : {}) }
    execFile(
      BIN,
      [tool, JSON.stringify(payload)],
      { cwd, timeout: TIMEOUT, maxBuffer: MAXBUF, env: process.env },
      (err, stdout, stderr) => {
        let parsed = null
        try {
          const o = JSON.parse(stdout)
          if (o && typeof o === "object") parsed = o
        } catch {
          /* not JSON */
        }
        if (err) {
          if (parsed) return reject(new Error(parsed.error?.message ?? `oc-ops ${tool} failed`))
          return reject(new Error(`oc-ops ${tool} failed: ${stderr || err.message}`))
        }
        if (!parsed) return reject(new Error(`oc-ops ${tool} returned invalid output`))
        resolve(parsed)
      },
    )
  })
}

function fmtItem(item) {
  const parts = []
  for (const k of ["path", "line", "name", "text", "hex"]) if (item[k] !== undefined) parts.push(String(item[k]))
  if (item.count !== undefined) parts.push(`count:${item.count}`)
  if (item.job_id !== undefined) parts.push(`job:${item.job_id}`)
  if (item.done !== undefined) parts.push(`done:${item.done}`)
  return parts.length ? parts.join(": ") : JSON.stringify(item).slice(0, 2000)
}

function render(resp) {
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
  for (const item of resp.items ?? []) {
    if (typeof item === "string") lines.push(item)
    else if (item && typeof item === "object") lines.push(fmtItem(item))
  }
  if (resp.truncated) lines.push("(output truncated — use offset/limit to page)")
  if (resp.nextOffset !== undefined && !resp.complete) lines.push(`nextOffset: ${resp.nextOffset}`)
  return lines.join("\n") || "(no items)"
}

const obj = (properties, required = []) => ({ type: "object", properties, required, additionalProperties: false })
const str = (description) => ({ type: "string", description })
const optStr = (description) => ({ type: "string", description })
const int = (description, minimum = 0) => ({ type: "integer", description, minimum })
const bool = (description) => ({ type: "boolean", description })
const strArr = (description) => ({ type: "array", items: { type: "string" }, description })
const enumStr = (values, description) => ({ type: "string", enum: values, description })

const TOOLS = [
  {
    name: "ops_grep",
    go: "grep",
    description:
      "Search file contents under path, summary-first. Prefer outputMode=summary for counts, count for per-file counts, files_with_matches for paths, content for line bodies. glob[] filters with ! to exclude. Bounded: use offset/limit to page.",
    input: obj(
      {
        pattern: str("Go regexp to search for"),
        path: optStr("Search root (defaults to cwd)"),
        glob: strArr("Include filters; ! excludes"),
        outputMode: enumStr(["content", "files_with_matches", "count", "summary"], "Output shape; summary is default"),
        caseInsensitive: bool("Case insensitive"),
        lineNumbers: bool("Default true"),
        beforeContext: int("Context lines before", 0),
        afterContext: int("Context lines after", 0),
        offset: int("Page offset", 0),
        limit: int("Page size", 1),
      },
      ["pattern"],
    ),
  },
  {
    name: "ops_glob",
    go: "glob",
    description:
      "Find files under path by explicit patterns. Prefer pattern[] batches with ! exclusions. Choose paths (default) or details output. Use for discovering paths, not reading.",
    input: obj(
      {
        path: optStr("Search root (defaults to cwd)"),
        pattern: strArr("Glob patterns; ! excludes"),
        outputMode: enumStr(["paths", "details"], "Default paths"),
        sort: enumStr(["path", "modified"], "Sort order"),
        offset: int("Page offset", 0),
        limit: int("Page size", 1),
      },
      ["pattern"],
    ),
  },
  {
    name: "ops_inspect",
    go: "inspect",
    description:
      "Read bounded UTF-8 text lines (or hex view) from a single file relative to cwd. Directories are rejected — use ops_glob or ops_grep. Use offset/limit for continuation.",
    input: obj(
      {
        path: str("File path relative to cwd"),
        offset: int("1-based start line"),
        limit: int("Max lines", 1),
        view: enumStr(["text", "hex"], "View mode"),
      },
      ["path"],
    ),
  },
  {
    name: "ops_replace",
    go: "replace",
    description:
      "Deterministic UTF-8 replacement under path. Dry-run by default (observe only). Pass apply:true to write. Check dry-run results before applying.",
    input: obj(
      {
        path: optStr("Root (defaults to cwd)"),
        pattern: str("Go regexp to match"),
        replacement: str("Literal replacement text"),
        glob: strArr("Include filters; ! excludes"),
        maxReplacements: int("Max replacements", 1),
        apply: bool("Writes when true; otherwise observe only"),
        dryRun: bool("Dry run"),
      },
      ["pattern", "replacement"],
    ),
  },
  {
    name: "ops_map",
    go: "map",
    description:
      "Produce a compact repository structure map and symbol index. Use when exploring an unfamiliar repo or to orient before deep reads. mode: tree | symbols | both.",
    input: obj({
      path: optStr("Repo root (defaults to cwd)"),
      depth: int("Directory depth", 1),
      mode: enumStr(["tree", "symbols", "both"], "Default both"),
      glob: strArr("Include filters; ! excludes"),
      offset: int("Page offset", 0),
      limit: int("Page size", 1),
    }),
  },
  {
    name: "ops_bg_start",
    go: "bg_start",
    owner: true,
    description:
      "Start a long-running command as a managed background job. Returns a job_id. Use for Gradle/NDK builds, sims, and long processes. Only jobs started by this session can be read or killed.",
    input: obj(
      {
        command: str("Bash command to run"),
        cwd: optStr("Working directory (defaults to session cwd)"),
        label: optStr("Human label"),
        timeoutMs: int("Max lifetime in ms", 1),
      },
      ["command"],
    ),
  },
  {
    name: "ops_bg_output",
    go: "bg_output",
    owner: true,
    description: "Read output of a job started by this session. done indicates whether it finished.",
    input: obj({ job_id: str("Job id returned by ops_bg_start") }, ["job_id"]),
  },
  {
    name: "ops_bg_list",
    go: "bg_list",
    owner: true,
    description: "List background jobs started by this session.",
    input: obj({}),
  },
  {
    name: "ops_bg_kill",
    go: "bg_kill",
    owner: true,
    description: "Terminate a background job started by this session.",
    input: obj({ job_id: str("Job id returned by ops_bg_start") }, ["job_id"]),
  },
  {
    name: "ops_batch",
    go: "batch",
    owner: true,
    description:
      "Preflight and verify batch copy/move/remove. action=plan requires operations and creates a no-effect audit plan; execute with action=resume plus id. action=run executes immediately. Use ops_result page to read details.",
    input: obj(
      {
        action: enumStr(["plan", "run", "resume"], "plan preflights; run validates+executes; resume executes existing plan by id"),
        operations: {
          type: "array",
          description: "1..512 operations",
          items: obj(
            {
              op: enumStr(["copy", "move", "remove"], "Operation kind"),
              path: str("Relative path under root"),
              to: optStr("Destination; required for copy/move"),
            },
            ["op", "path"],
          ),
        },
        operationsRef: optStr("ops_result id holding operation rows"),
        root: optStr("Root directory (defaults to session cwd)"),
        protect: strArr("Paths never touched; max 128"),
        id: optStr("Plan id to resume"),
      },
      ["action"],
    ),
  },
  {
    name: "ops_result",
    go: "result",
    owner: true,
    description:
      "Bounded session-owned result references: put object rows, page with equals/fields filters and byte budgets, release results. 24h expiry. References are observations, never authority.",
    input: obj(
      {
        action: enumStr(["put", "page", "release"], "put stores rows; page reads; release frees"),
        items: { type: "array", description: "Rows for put; 1..10000 objects", items: { type: "object", additionalProperties: true } },
        id: optStr("Reference id for page/release"),
        offset: int("Page offset", 0),
        limit: int("Max rows per page", 1),
        maxBytes: int("Byte budget per page", 512),
        fields: strArr("Project only these top-level fields"),
        equals: { type: "object", additionalProperties: true, description: "Filter: rows must match these top-level fields" },
        root: optStr("Scoping root (informational)"),
      },
      ["action"],
    ),
  },
  {
    name: "ops_android_env",
    go: "android_env",
    description:
      "Resolve and report the Android/NDK/JAVA toolchain environment. Use before Android builds to confirm ANDROID_HOME, NDK, and JAVA_HOME are set.",
    input: obj({}),
  },
  {
    name: "ops_android_apk",
    go: "android_apk",
    description:
      "Inspect an APK's metadata (package name, version, sdk versions, permissions) via aapt2 badging. Use for quick APK identification before a full decompile.",
    input: obj({ apk: str("Path to the APK") }, ["apk"]),
  },
  {
    name: "ops_android_dex",
    go: "android_dex",
    description: "List dex entries in an APK (compact). For full class/method analysis use jadx or dex2jar.",
    input: obj({ apk: str("Path to the APK") }, ["apk"]),
  },
]

const ROUTING = [
  "# oc-ops tool routing",
  "",
  "| Task | Tool |",
  "| --- | --- |",
  "| Search contents | ops_grep (summary-first) |",
  "| Find paths | ops_glob |",
  "| Read a file / hex | ops_inspect |",
  "| Batch replacement | ops_replace (apply:true to write) |",
  "| Explore an unfamiliar repo | ops_map |",
  "| Resolve Android/NDK/JAVA env | ops_android_env |",
  "| Inspect an APK | ops_android_apk |",
  "| Long-running builds | ops_bg_start / ops_bg_output / ops_bg_list / ops_bg_kill |",
  "| Programmatic batch copy/move/remove | ops_batch (plan -> resume) |",
  "| Bounded result references | ops_result (put/page/release) |",
  "",
  "- ops_* tools are token-efficient and bounded: prefer them for searches and exploration.",
  "- On a tool error, correct the arguments and retry; do not switch to a shell workaround.",
  "- Use ops_grep summary/count/files_with_matches to avoid large output.",
].join("\n")

export default {
  id: "oc-ops",
  async setup(ctx) {
    const cwd = ctx?.location?.directory || process.cwd()

    // Register all ops_* tools.
    await ctx.tool.transform((ed) => {
      for (const t of TOOLS) {
        ed.add({
          name: t.name,
          description: t.description,
          input: t.input,
          execute: async (input, context) => {
            try {
              const resp = await invoke(t.go, input, cwd, t.owner ? context?.sessionID : undefined)
              return { content: render(resp) }
            } catch (err) {
              return { content: `[oc-ops ${t.name} error] ${String(err?.message ?? err)}` }
            }
          },
        })
      }
    })

    // Shell env injection (best effort).
    try {
      let android = {}
      try {
        const resp = await invoke("android_env", {}, cwd)
        android = resp?.items?.[0] ?? {}
      } catch {
        /* ignore */
      }
      await ctx.shell.hook("create.before", (event) => {
        const env = event?.env
        if (!env) return
        if (android.ANDROID_HOME) env.ANDROID_HOME = android.ANDROID_HOME
        if (android.ANDROID_NDK_HOME) {
          env.ANDROID_NDK_HOME = android.ANDROID_NDK_HOME
          env.NDK_HOME = android.ANDROID_NDK_HOME
        }
        if (android.JAVA_HOME) env.JAVA_HOME = android.JAVA_HOME
        if (android.NDK_TOOLCHAIN) env.NDK_TOOLCHAIN = android.NDK_TOOLCHAIN
        env.NO_COLOR = env.NO_COLOR ?? "1"
        env.PAGER = env.PAGER ?? "cat"
        env.GIT_PAGER = env.GIT_PAGER ?? "cat"
      })
    } catch {
      /* shell hook API unavailable — env can still come from the shell */
    }

    // System-prompt routing section (best effort).
    try {
      await ctx.session.hook("context", (event) => {
        if (Array.isArray(event?.system)) event.system.push({ type: "text", text: ROUTING })
      })
    } catch {
      /* session hook API unavailable */
    }
  },
}
