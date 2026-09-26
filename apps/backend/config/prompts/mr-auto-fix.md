You are continuing work on a merge request because Kandev detected new pipeline or discussion feedback.

Focus on the current merge request feedback provided in the task message:

{{mr.feedback}}

- Fix failing pipeline jobs and actionable unresolved discussion comments.
- Prioritize feedback marked as new or changed since the last automated fix round.
- When an actionable discussion comment has been addressed, reply to that thread with the fix summary and resolve the addressed discussions so they do not keep the MR blocked.
- Preserve unrelated work and avoid broad refactors.
- Run the narrowest relevant verification commands first, then broader checks if needed.
- Do not merge the merge request. Kandev handles auto-merge separately when the MR is ready.

First classify the new MR feedback as actionable or non-actionable.

If the new feedback is not actionable, do not modify files, do not commit, and do not push.
Non-actionable feedback includes summaries, status updates, no-finding reports, duplicated or
previously addressed comments, rate-limit notices, and review diagnostics that do not request a
concrete code or test change. In that case, reply only with a short summary that there is nothing actionable to address.
Pending-only MR snapshots are also non-actionable: when `failed_jobs: []` and there are zero
unresolved discussions, but the pipeline is still queued or running, do not modify files, do not run
local verification, do not commit, and do not poll indefinitely. Reply that the pipeline is still in
progress and include the pending job names if they were provided.

Only make code changes when there is a concrete failing job, actionable discussion comment, or
reproducible issue that needs a fix. Do not push a commit merely to acknowledge feedback.

When you finish, summarize what changed and which verification commands you ran.
