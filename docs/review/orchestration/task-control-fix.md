# Orchestrator task control repair

Workspace conversations advertised private-conversation tools and a mandatory
objective contract that their runtime could not satisfy. Even direct task
management lacked edit, move, archive and delete actions. The existing auth
middleware repair allowed board reads but did not resolve these failures.

## Implemented behavior

- Derive MCP discovery scope from the server-owned conversation surface.
  Workspace discovery and capability reads share one control catalog.
- Create design/execute tasks directly from a workspace conversation, without
  an owner binding, objective or user-supplied Kandev API key.
- Edit title, description, priority and task parent; move between native workflow
  steps; archive and delete through the existing task services. Existing
  assignment, adoption, start, stop, worker messages and status controls remain
  available and are described explicitly to the model.
- Read workspace Orchestrator memory without private-conversation setup.
- Report status codes for empty HTTP errors instead of empty tool failures.
- Avoid adding an objective delivery link after private task deletion.
- Apply the existing managed Claude broker allowlist to workspace conversations.
  A synthetic provider trial caught permission prompts for native broker reads;
  only the configured `kandev_assistant` tools are preapproved. General permission
  bypass stays off and provider account environment remains available.

Task services continue to publish board events and perform cleanup. Workspace
and conversation boundaries, active-run credentials, active-session move rules,
manual-move configuration and required review decisions remain enforced.
Deletion does not supply consent to discard uncommitted work.

## Validation

Tests first reproduced the blocked creation, missing actions, misleading tool
catalog, empty errors and post-deletion objective link. Regression coverage now
includes signed runtime scope enforcement, stale-run rejection, foreign workspace
and conversation rejection, review/move-policy guards, native task lifecycle and
an HTTP-to-SQLite create/edit/move/status/archive/delete integration test.

The agentctl, runtime configuration, orchestration, backend application and auth
middleware test suites pass. Changed-code Go lint reports no issues. All 61
public-doc validator tests and validation of 48 published pages pass.

All fixtures use synthetic requests. No live prompt or conversation transcript
is included in this change.

## Scope and remaining limits

Workspace task operations retain their existing native API contract: omit
`operation_id`, `expected_intent_revision` and `objective_id`. They have no
private-conversation operation receipt and are not automatically retried after
uncertain delivery. The model must inspect native evidence before retrying.

Private-conversation objective receipts, linked-workspace grants and maintenance
remain private-scoped capabilities; they are not prerequisites for operating the
workspace board. This repair changes backend task controls and tool discovery;
it does not add a separate Orchestrator UI or require a database migration.

The provider policy uses the existing qualified Claude ACP version and
[SDK MCP allowlist](https://code.claude.com/docs/en/agent-sdk/mcp). It does not
turn on blanket auto-approval or approve worker permission requests.

## Actual provider qualification

The final packaged candidate passed a three-turn managed Claude ACP trial in an
isolated, synthetic workspace. The first turn read capabilities and memory,
created an unassigned task, edited title/description/priority and moved its board
column. The resumed conversation archived the task in turn two and deleted it
in turn three. Database assertions verified each result. No permission request
was emitted, and no synthetic delivery task remained after cleanup. The provider
credential link was removed when the fixture stopped.

See the [sanitized provider receipt](task-control-provider-receipt.json). This
qualifies the packaged chat/runtime path; it is not a transcript of live user work.

## Live deployment

Candidate `0.94.0-orchestration.20260918.sha2eaf7a892` is deployed. A verified cold
backup and the previous bundle/service configuration are retained privately.
Startup health reports the expected version, unauthenticated workspace access
remains denied, database integrity and foreign keys pass, and retained row counts
match. No live user prompt was replayed or copied into this packet.

See the [sanitized deployment receipt](task-control-live-receipt.json). Refresh the
UI and send a new message in the existing Orchestrator conversation to use the
new runtime and tool discovery.
