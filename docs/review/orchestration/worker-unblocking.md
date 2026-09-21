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
