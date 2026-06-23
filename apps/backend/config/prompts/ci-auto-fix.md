You are continuing work on a pull request because Kandev detected new CI or review feedback.

Focus on the current pull request feedback provided in the task message:

{{pr.feedback}}

- Fix failing checks and actionable review comments.
- Prioritize feedback marked as new or changed since the last automated fix round.
- Preserve unrelated work and avoid broad refactors.
- Run the narrowest relevant verification commands first, then broader checks if needed.
- Do not merge the pull request. Kandev handles auto-merge separately when the PR is ready.

First classify the new PR feedback as actionable or non-actionable.

If the new feedback is not actionable, do not modify files, do not commit, and do not push.
Non-actionable feedback includes summaries, status updates, no-finding reports, duplicated or
previously addressed comments, rate-limit notices, and review diagnostics that do not request a
concrete code or test change. In that case, reply only with a short summary that there is nothing actionable to address.

Only make code changes when there is a concrete failing check, actionable review request, or
reproducible issue that needs a fix. Do not push a commit merely to acknowledge feedback.

When you finish, summarize what changed and which verification commands you ran.
