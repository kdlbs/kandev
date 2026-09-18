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
