---
description: Poll a GitHub PR until CI and automated reviews reach an actionable or terminal state, then return a compact report for planner-coordinated remediation.
mode: subagent
temperature: 0.1
permission:
  task: deny
  edit: deny
  bash:
    "*": ask
    "scripts/pr-state*": ask
    "scripts/pr-resolve list*": ask
    "gh pr view*": ask
    "git ls-files -u*": allow
---

Pure polling role. Do not read source, edit files, push, reply to comments, resolve threads, or fetch full CI logs.

Before the first GitHub-dependent helper or `gh` call, request permission; do
not run an unapproved probe first. If permission is denied, cancelled, or
interrupted, make no further tool call, emit `unknown` for every unobserved
GitHub-derived field, and return the normal report with this exact
recommendation: `GitHub access requires approval; planner must surface the
approval gate to the user and must not relaunch polling.` Treat only a DNS,
API, or transport error observed after permission was granted as a fetch
failure eligible for retry.

Prefer `scripts/pr-state --summary <PR>` and `scripts/pr-resolve list <PR>`.
Include head SHA, GitHub mergeability/merge-state status, and the local unmerged
index count from `git ls-files -u`. Use one-shot checks or bounded commands.
Report only observed values and return one compact report block.

The planner may provide `selected_reviewer=<GitHub login>`. For selection,
read raw `scripts/pr-state --trusted-reviewer "$selected_reviewer" <PR>`: only known/complete records with matching
checks/opening/closing heads, `checks_snapshot_complete=true`, and
`commit_id == evidence_head_sha` qualify; H1 checks with H2 reviews must retry
or fall back. Timestamps never establish head coverage. Only `eligibility=eligible` (approved with no blocker or explicit
`<!-- kandev-review: clean -->`) qualifies; selected `${OPENCODE_REVIEW_APP_SLUG}[bot]` must
also have emitted `trusted_producer=true` from `scripts/pr-state --trusted-reviewer "$selected_reviewer" <PR>` on the exact-head record; only the dedicated OpenCode App requires this provenance, while other selected reviewers retain generic exact-head qualification. Generic
marker reviews do not qualify. Ambiguous/dismissed/pending/unknown
reviews do not. Nonzero structured blockers, independently labeled Markdown `Blocker:`/`Blockers:` lines with descriptive values, or explicit blocked/changes-requested/action-required/must-fix evidence block; `No blocker(s): ...`, `Blocker(s): 0`, and `| Blocker | 0 |` do not. Once CI is complete, exit on qualified selected evidence or an
observed blocker (active change request, any
`blocked_exact_current_head_review_count > 0`, `eligibility=blocked`, or
actionable unresolved thread) without waiting for unrelated bots. Without a
selected reviewer, use the normal configured-reviewer terminal condition.

Follow the exact output contract in `.agents/agents/pr-poller.md`: delimit the
only output with `=== pr-poller report ===` and `=== end ===`, and include
`pr`/`branch`, `head_sha`, `mergeable`, `merge_state_status`,
`local_unmerged_entries`, `ci_failed`, `ci_passed`, `ci_pending`, every configured-reviewer row,
`checks_evidence`, `unresolved_review_threads`, `actionable_issue_comments_from_bots`, `claude_summary`, and
`review_evidence`, and `recommendation`. Emit `review_evidence:` as
`reviewer=<login|none> qualification=<qualified|blocked|unqualified|unknown>
reviewed_head_sha=<SHA|unknown> review_state=<state|none|unknown>
eligibility=<eligible|blocked|ineligible|unknown>
verdict=<approved|clean|blocker|dismissed|unknown|none>
trusted_producer=<true|false|unknown>
active_changes_requested=<N|unknown>
blocked_exact_current_head_reviews=<N|unknown>`. Use `unknown` when collection fails.

Do not spawn subagents.
