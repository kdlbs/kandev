# Read worker evidence and resolve authorized blockers

The workspace Orchestrator can inspect complete worker results and handle native
one-time permission requests for authorized work.

Task details previously included full task/session objects, including runtime
snapshots, despite clipping individual message excerpts. A long runtime snapshot
could overflow the provider tool-result limit. Details now use compact projections
and honor `include_result=false`. `task_content` supplies paged requirements,
message previews, complete message chunks, answered clarification responses and
shell failure output. Cross-workspace, task/session and message ownership checks
apply before export; runtime snapshots and unrelated metadata are excluded.

`task_permissions` exposes live native permission requests. `manage_task` adds
`session_mode` for default/manual, accept-edits and automatic permission modes,
and `resolve_permission` for an exact live allow-once or reject-once choice.
Mode changes are scoped to one session and survive restart. Provider errors are
returned rather than reported as success. Persistent permission grants and bypass
modes are unavailable. Resolution uses native request-generation validation,
durable audit claims and delivery finalization.

An automatic-classifier rejection does not create a pending native permission
request. The recovery sequence is: inspect the failure and the user's scope,
select manual review for that session, resume the worker on the authorized action,
then inspect and resolve the actual provider request. Worker results do not grant
new authority. Existing human-only private-assistant attention controls and linked
workspace export rules remain separate.

Regression coverage uses generic examples: oversized runtime metadata, full
multi-page results, foreign messages/sessions/cursors, signed workspace scope,
expired runs, offered one-time options, unsupported bypass modes, native mode
delivery errors and offline mode persistence. Live prompts and private task data
are not part of this review packet.

Cold worker starts and resumed sessions use a two-minute broker deadline instead
of the ordinary thirty-second read deadline. Requests are sent once; an uncertain
response still requires inspecting native state before any retry. Read deadlines
and the shared HTTP client remain unchanged. Result previews select agent-authored
messages so a follow-up prompt does not displace the worker's output.

Validation: backend application, orchestration and native orchestrator package
suites passed with the race detector. Focused broker, native mode-persistence,
result-pagination, permission-choice and signed-scope tests passed. A virtual-time
regression reproduces the former thirty-second cold-resume failure and verifies
one successful request after thirty-one seconds; normal reads still time out.
The public-doc validator passed all 48 pages. Normal commit hooks passed without
bypasses, including architecture and Go lint.

## Commit receipts

| Field | Worker inspection and controls | Resume and result follow-up |
| --- | --- | --- |
| parent_sha | `1fb1f7c876f7fd5e70f86ef47dbd833b614025b5` | `1a613eb9a34144a403221906455225c593a0117e` |
| commit_sha | `1a613eb9a34144a403221906455225c593a0117e` | `4fda12610b7c4e519e798922fae149dbe5351a5e` |
| pre_commit_hook | active | active |
| commit_msg_hook | active | active |
| bypass | false | false |
| commit_result | pass | pass |
| worktree after commit | clean | clean |

For both implementation commits, `harness-lint`, `architecture-lint`, `gofmt`,
`go-lint`, `public-copy-em-dash` and `commitlint` passed. `specification-lint`,
`prettier-format`, `web-lint`, `i18n-new-code` and `e2e-sleep-ratchet` were skipped
because their file filters did not match. The repeated commit-message-stage
checks also passed, with Go checks skipped at that stage.

## Live loop validation

The first candidate was deployed with a verified cold backup. Health, web
readiness, authentication enforcement, retained data and SQLite integrity checks
passed. Through the live Orchestrator conversation, the model retrieved bounded
worker evidence, read the remainder of a long result, inspected live permissions,
changed one worker's permission mode, and resumed the same session. The worker
completed its authorized validation and data update. Native task completion
queued a callback, and the Orchestrator read the new result and reported it in
the central conversation without another user message.

The resumed action was permitted by the provider's existing manual-mode rules;
no new native permission request needed approval in this live run. Exact
one-time permission resolution is covered by native service tests and the new
scope/option regressions, rather than claimed as exercised by this live action.

The first resume exceeded the old thirty-second broker deadline. The Orchestrator
inspected native state, confirmed that execution had begun and did not send a
duplicate prompt. That observed failure drove the cold-resume regression and
follow-up fix above. No live transcript, prompts, credential material or private
worker task identifiers are included in these receipts.

Final deployed candidate: `0.94.0-orchestration.20260921.sha4fda12610b7c`. The second cutover
also passed verified cold backup, health and web readiness, authentication,
retained-row, SQLite integrity and foreign-key checks.

The follow-up chat-state changes were built from the public fork branch
`Corey-Fogg/kandev:feat/workspace-orchestration` at commit
`aa0407394653899dde226f719845e3acfcc541ae` and deployed as
`0.94.0-orchestration.20260921.shaaa0407394653`. The local deployment remote now
points at that public fork; the former private repository remains available as
the `private` remote for historical review. The fork and upstream review are
linked from [kdlbs/kandev#3853](https://github.com/kdlbs/kandev/pull/3853).

On the final candidate, a second live conversation asked the same worker for a
read-only confirmation of its completed results. The broker's single `message`
request completed successfully; the worker returned the confirmation, and the
Orchestrator read and reported it in the central conversation. All tool calls in
that follow-up completed successfully, and both the user-triggered run and native
completion-callback run finished with no error. No migration or data update was
repeated. The actual resumed call finished within ten seconds; the longer
thirty-one-second regression is verified separately with virtual time.
