You are continuing work on a pull request because Kandev detected new CI or review feedback.

Focus on the current pull request feedback provided in the task message:

{{pr.feedback}}

- Fix failing checks and actionable review comments.
- Prioritize feedback marked as new or changed since the last automated fix round.
- When an actionable PR review comment has been addressed, reply to that thread with the fix summary and resolve the addressed PR review threads so they do not keep the PR blocked.
- Preserve unrelated work and avoid broad refactors.
- Run the narrowest relevant verification commands first, then broader checks if needed.
- Do not merge the pull request. Kandev handles auto-merge separately when the PR is ready.

First classify the new PR feedback as actionable or non-actionable.

If the new feedback is not actionable, do not modify files, do not commit, and do not push.
Non-actionable feedback includes summaries, status updates, no-finding reports, duplicated or
previously addressed comments, rate-limit notices, and review diagnostics that do not request a
concrete code or test change. In that case, reply only with a short summary that there is nothing actionable to address.
Pending-only PR snapshots are also non-actionable: when `failed_checks: []` and there are zero
unresolved review threads, but checks are still queued or running, do not modify files, do not run
local verification, do not commit, and do not poll indefinitely. Reply that CI is still in progress
and include the pending check names if they were provided.

Only make code changes when there is a concrete failing check, actionable review request, or
reproducible issue that needs a fix. Do not push a commit merely to acknowledge feedback.

When you finish, summarize what changed and which verification commands you ran.
