import { tool } from "@opencode-ai/plugin"
import path from "path"

export default tool({
  description:
    "Read one regular UTF-8 file from the immutable pull request head. Use this instead of repository read tools for PR-head contents.",
  args: {
    path: tool.schema
      .string()
      .max(4096)
      .describe("Strict repository-relative path at the pull request head"),
  },
  async execute(args, context) {
    const helper = path.join(context.worktree, ".opencode-walkthrough", "read-file")
    const sourceEnv = globalThis.process.env
    const headSha = sourceEnv.HEAD_SHA ?? ""
    const childEnv = Object.fromEntries(
      ["PATH"]
        .filter((name) => sourceEnv[name] !== undefined)
        .map((name) => [name, sourceEnv[name] as string]),
    )
    const child = Bun.spawn(
      ["python3", helper, "--repo", context.worktree, "--head-sha", headSha, "--path", args.path],
      {
        cwd: context.worktree,
        env: childEnv,
        stdout: "pipe",
        stderr: "pipe",
      },
    )
    const [status, stdout, stderr] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
      new Response(child.stderr).text(),
    ])
    if (status !== 0) {
      return `PR file read rejected. Choose a regular UTF-8 file from the changed-file list.\n${stderr || stdout}`
    }
    return stdout
  },
})
