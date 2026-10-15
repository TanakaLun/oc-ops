// Background job tools: ops_bg_start, ops_bg_output, ops_bg_list, ops_bg_kill.
// Long builds/sims run as managed jobs instead of burning a foreground call.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

const s = tool.schema

export function backgroundTools(binaryPath) {
  return {
    ops_bg_start: tool({
      description:
        "Start a long-running command as a managed background job. Returns a job_id. Use for Gradle/NDK builds, sims, and long processes that would otherwise occupy the call. Only jobs started by this session can be read or killed.",
      args: {
        command: s.string().describe("Bash command to run"),
        cwd: s.string().optional().describe("Working directory (defaults to session cwd)"),
        label: s.string().optional().describe("Human label"),
        timeoutMs: s.number().int().min(1).optional().describe("Max lifetime in ms"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "bg_start", args, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops bg_start error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_bg_output: tool({
      description:
        "Read output of a job started by this session. Use job_id from ops_bg_start. done indicates whether it finished.",
      args: {
        job_id: s.string().describe("Job id returned by ops_bg_start"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "bg_output", args, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops bg_output error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_bg_list: tool({
      description: "List background jobs started by this session.",
      args: {},
      execute: async (_args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "bg_list", {}, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops bg_list error] ${String(err?.message ?? err)}`
        }
      },
    }),

    ops_bg_kill: tool({
      description: "Terminate a background job started by this session.",
      args: {
        job_id: s.string().describe("Job id returned by ops_bg_start"),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "bg_kill", args, { cwd: ctx.directory, owner: ctx.sessionID })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops bg_kill error] ${String(err?.message ?? err)}`
        }
      },
    }),
  }
}
