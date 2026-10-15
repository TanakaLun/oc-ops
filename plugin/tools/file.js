// File tools: ops_grep, ops_glob, ops_inspect, ops_replace.
// These are token-efficient: bounded, summary-first, strict-argument.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

const s = tool.schema

export function fileTools(binaryPath) {
  return {
    ops_grep: tool({
      description:
        "Search file contents under path, summary-first. Prefer outputMode=summary for counts, count for per-file counts, files_with_matches for paths, content for line bodies. Use glob[] to filter and ! to exclude. Bounded: use offset/limit to page. Skip symlinks, .git, node_modules, target.",
      args: {
        pattern: s.string().describe("Go regexp to search for"),
        path: s.string().optional().describe("Search root (defaults to cwd)"),
        glob: s.array(s.string()).optional().describe("Include filters; ! excludes"),
        outputMode: s.enum(["content", "files_with_matches", "count", "summary"]).optional().describe("Output shape; summary is default"),
        caseInsensitive: s.boolean().optional(),
        lineNumbers: s.boolean().optional().describe("Default true"),
        beforeContext: s.number().int().min(0).max(32).optional(),
        afterContext: s.number().int().min(0).max(32).optional(),
        offset: s.number().int().min(0).optional(),
        limit: s.number().int().min(1).max(10000).optional(),
      },
      execute: (args, ctx) => run(binaryPath, "grep", args, ctx),
    }),

    ops_glob: tool({
      description:
        "Find files under path by explicit patterns. Prefer pattern[] batches with ! exclusions. Choose paths (default) or details output. Use for discovering paths, not reading.",
      args: {
        path: s.string().optional().describe("Search root (defaults to cwd)"),
        pattern: s.array(s.string()).describe("Glob patterns; ! excludes"),
        outputMode: s.enum(["paths", "details"]).optional().describe("Default paths"),
        sort: s.enum(["path", "modified"]).optional(),
        offset: s.number().int().min(0).optional(),
        limit: s.number().int().min(1).max(10000).optional(),
      },
      execute: (args, ctx) => run(binaryPath, "glob", args, ctx),
    }),

    ops_inspect: tool({
      description:
        "Read bounded UTF-8 text lines (or hex view) from a single file relative to cwd. Directories are rejected — use ops_glob or ops_grep. Use offset/limit for continuation.",
      args: {
        path: s.string().describe("File path relative to cwd"),
        offset: s.number().int().min(1).optional().describe("1-based start line"),
        limit: s.number().int().min(1).max(10000).optional(),
        view: s.enum(["text", "hex"]).optional(),
      },
      execute: (args, ctx) => run(binaryPath, "inspect", args, ctx),
    }),

    ops_replace: tool({
      description:
        "Deterministic UTF-8 replacement under path. Dry-run by default (observe only). Pass apply:true to write. Check dryRun results before applying. Use host edit for precise single edits.",
      args: {
        path: s.string().optional().describe("Root (defaults to cwd)"),
        pattern: s.string().describe("Go regexp to match"),
        replacement: s.string().describe("Literal replacement text"),
        glob: s.array(s.string()).optional().describe("Include filters; ! excludes"),
        maxReplacements: s.number().int().min(1).max(1000000).optional(),
        apply: s.boolean().optional().describe("Writes when true; otherwise observe only"),
        dryRun: s.boolean().optional(),
      },
      execute: (args, ctx) => run(binaryPath, "replace", args, ctx),
    }),
  }
}

async function run(binaryPath, toolName, args, ctx) {
  try {
    const resp = await invokeCLI(binaryPath, toolName, args, { cwd: ctx.directory })
    return renderResponse(resp)
  } catch (err) {
    return `[oc-ops ${toolName} error] ${String(err?.message ?? err)}`
  }
}
