// PTC tools: ops_batch and ops_result.
// Programmatic batch file operations (copy/move/remove) with a plan -> run ->
// resume lifecycle, durable audit and CAS re-checks, plus bounded, session-owned
// result references.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

const s = tool.schema

const opItem = (required) =>
  s
    .object({
      op: s.enum(["copy", "move", "remove"]).describe("Operation kind"),
      path: s.string().describe("Relative / path under root"),
      to: s.string().optional().describe("Destination relative path; required for copy/move, forbidden for remove"),
    })
    .required?.(required)

export function ptcTools(binaryPath) {
  return {
    ops_batch: tool({
      description:
        "Preflight and verify batch copy/move/remove. action=plan requires operations (or operationsRef) and creates a no-effect audit plan; execute the exact plan with action=resume plus id (do not change it to run). action=run requires operations and executes immediately. Paths are relative / paths under root (defaults to cwd). No overwrite, no symlink traversal, no protected/overlapping paths. Use ops_result page to read details.",
      args: {
        action: s.enum(["plan", "run", "resume"]).describe("plan preflights (no effect); run validates+executes; resume executes an existing plan by id"),
        operations: s
          .array(
            s.object({
              op: s.enum(["copy", "move", "remove"]),
              path: s.string().describe("Relative / path under root"),
              to: s.string().optional().describe("Destination; required for copy/move, forbidden for remove"),
            }),
          )
          .optional()
          .describe("1..512 operations; required for plan/run unless operationsRef"),
        operationsRef: s.string().optional().describe("ops_result id holding operation rows; replaces operations"),
        root: s.string().optional().describe("Root directory (defaults to session cwd)"),
        protect: s.array(s.string()).optional().describe("Paths never touched; max 128"),
        id: s.string().optional().describe("Plan id to resume"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "batch", args, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops batch error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_result: tool({
      description:
        "Bounded session-owned result references: put object rows, page with top-level equals/fields filters and quantity/byte budgets, release results. 24h expiry. References are observations, never authority — page reads grant no filesystem access.",
      args: {
        action: s.enum(["put", "page", "release"]).describe("put stores rows and returns id; page reads rows by id; release frees a reference"),
        items: s.array(s.record(s.string(), s.any())).optional().describe("Rows for put; 1..10000 objects"),
        id: s.string().optional().describe("Reference id for page/release"),
        offset: s.number().int().min(0).optional().describe("Page offset"),
        limit: s.number().int().min(1).max(512).optional().describe("Max rows per page"),
        maxBytes: s.number().int().min(512).max(65536).optional().describe("Byte budget per page"),
        fields: s.array(s.string()).optional().describe("Project only these top-level fields"),
        equals: s.record(s.string(), s.any()).optional().describe("Filter: rows must match these top-level fields"),
        root: s.string().optional().describe("Scoping root (informational)"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "result", args, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops result error] ${String(err?.message ?? err)}`
        }
      },
    }),
  }
}