// Tool factory: one opencode tool wrapping a call to the oc-ops Go backend.
import { tool } from "@opencode-ai/plugin"
import { invokeCLI, renderResponse } from "../cli.js"

// binaryPath and config are bound by the plugin instance.
export function makeTool({ binaryPath, toolName, description, argsSchema, component }) {
  return tool({
    description,
    args: argsSchema,
    async execute(args, context) {
      try {
        const resp = await invokeCLI(binaryPath, toolName, args, {
          cwd: context.directory,
          owner: context.sessionID,
        })
        return renderResponse(resp)
      } catch (err) {
        return `[oc-ops ${toolName} error] ${String(err?.message ?? err)}`
      }
    },
  })
}