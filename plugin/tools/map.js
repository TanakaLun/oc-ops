// ops_map — repository structure map + symbol index.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

const s = tool.schema

export function mapTool(binaryPath) {
  return {
    ops_map: tool({
      description:
        "Produce a compact repository structure map and symbol index. Use when exploring an unfamiliar repo or to orient before deep reads. tree emits one line per dir/file; symbols emits one line per declaration (path:line:name); both is default.",
      args: {
        path: s.string().optional().describe("Repo root (defaults to cwd)"),
        depth: s.number().int().min(1).max(12).optional().describe("Directory depth"),
        mode: s.enum(["tree", "symbols", "both"]).optional().describe("Default both"),
        glob: s.array(s.string()).optional().describe("Include filters; ! excludes"),
        offset: s.number().int().min(0).optional(),
        limit: s.number().int().min(1).max(2000).optional(),
      },
      execute: async (args, ctx) => {
        try {
          const resp = await invokeCLI(binaryPath, "map", args, { cwd: ctx.directory })
          return renderResponse(resp)
        } catch (err) {
          return `[oc-ops map error] ${String(err?.message ?? err)}`
        }
      },
    }),
  }
}
