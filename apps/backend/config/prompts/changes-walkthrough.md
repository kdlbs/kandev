Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.

Walkthrough requirements:
- Inspect the changed files yourself instead of relying on UI-provided paths or diff context.
- If this is a PR task or PR review task, compare the PR head against the PR base branch.
- Otherwise, compare against the task/repository base using the local task state (`git status`, `git diff`, `git diff --cached`, and cumulative diff context when available).
- For PR-only files, do not assume the PR head is checked out locally; anchor to the review diff when available, and avoid editor-only/current-worktree claims.
- The first walkthrough step must contextualize the whole change before explaining individual hunks.
- Anchor the first walkthrough step to the most representative changed line or changed line range available.
- Include an `ELI5:` line in the first walkthrough step text that explains the change in the simplest terms possible.
- Anchor steps to changed lines or changed line ranges whenever possible.
- Use `line_end` whenever a logical explanation spans multiple lines; prefer one range step over adjacent single-line steps.
- Keep each step concise and direct. Do not include a `Justification:` preamble.
- If a good local/review anchor is unavailable, omit that step instead of referencing a remote-only path.
