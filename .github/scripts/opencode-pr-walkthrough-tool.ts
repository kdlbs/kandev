import { tool } from "@opencode-ai/plugin"
import path from "path"

export default tool({
  description:
    "Validate and render a complete PR walkthrough JSON object. Retry with corrected JSON when this tool reports a renderer error.",
  args: {
    walkthrough: tool.schema
      .string()
      .max(2_000_000)
      .describe("The complete PR walkthrough JSON object as a string"),
  },
  async execute(args, context) {
    const helper = path.join(context.worktree, ".opencode-walkthrough", "render")
    const child = Bun.spawn(["python3", helper], {
      cwd: context.worktree,
      env: { ...globalThis.process.env },
      stdin: new Blob([args.walkthrough]),
      stdout: "pipe",
      stderr: "pipe",
    })
    const [status, stdout, stderr] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
      new Response(child.stderr).text(),
    ])
    if (status !== 0) {
      return `Renderer rejected the walkthrough. Correct the JSON and call this tool again.\n${stderr || stdout}`
    }
    return stdout.trim()
  },
})
