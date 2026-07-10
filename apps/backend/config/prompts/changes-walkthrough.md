Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.

Walkthrough requirements:
- Use only files listed below or files you verify exist in this task's local worktree/review diff.
- For PR-only files, do not assume the PR head is checked out locally; anchor to the review diff when available, and avoid editor-only/current-worktree claims.
- Anchor steps to changed lines or changed line ranges whenever possible.
- Use `line_end` whenever a logical explanation spans multiple lines; prefer one range step over adjacent single-line steps.
- Keep each step concise and direct. Do not include a `Justification:` preamble.
- If a good local/review anchor is unavailable, omit that step instead of referencing a remote-only path.

Available changed files:
{{changed_files}}
