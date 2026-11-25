### GitHub Operations
Skills use `gh` CLI by default. If a `gh` command fails (not installed, not authenticated, etc.), use whatever GitHub tools are available in the environment (MCP GitHub tools, API tools, etc.) to accomplish the same operation. The goal is the same — the tool may differ.

For multiline Markdown issue or PR bodies, use `gh ... --body-file <path>`; never send escaped
newlines through `--body`. After body edits, verify the live body because `gh pr edit` can no-op with
a Projects-classic warning; use REST PATCH if needed and refresh PR evidence because edits enqueue checks.

For PR review/fixup workflows, prefer the repo helpers before manually querying GitHub/GraphQL: `scripts/pr-await <PR>` to block until CI is terminal and get one report (do not manually poll `pr-state` on a timer in the primary conversation; preserve the documented `pr-poller` fallback when `pr-await` is unavailable), `scripts/pr-state --summary <PR>` for checks and unresolved-thread state, `scripts/pr-state --comment <comment_id>` for a full review-comment body, `scripts/pr-resolve list <PR>` for actionable unresolved review threads, and `scripts/pr-resolve reply <PR> <comment_id> <thread_id> "<body>"` to reply, resolve, and react in one call. In a shared worktree, a stale or locked `refs/remotes/origin/*` is not authoritative; read the remote SHA with `git ls-remote` and use an explicit `--force-with-lease=refs/heads/<branch>:<remote-sha>`.

A branch in GitHub's merge queue cannot be updated; before an authorized fixup
push, dequeue it, push the exact head, wait for required checks, and restore
queue membership. Otherwise report queue membership as a mutation blocker and
load `.agents/skills/pr-fixup/references/merge-queue.md`.

When a Kandev system message references an MCP tool that is not visible in the active tool list, use the runtime's tool discovery mechanism, such as `tool_search` when available, before falling back to a less specific workflow; if discovery exposes a qualified `mcp__kandev__<tool_name>` alias, call that active alias. Some task messaging and platform helpers are exposed on demand.
