---
created: 2026-10-12
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
legacy_specs: []
---

# Fix plan: resumed durable delivery

## Evidence and scope

Two reported resumed conversations emitted setup events before their first SQL
harness generation committed. An existing native resume token disabled the
generation-one bootstrap wait. The processor rejected ownership, cancelled
itself, and left its transport open. Subsequent events were discarded until
the stall watchdog stopped the agent, creating unknown-outcome recovery blocks.
Explicit Resume then failed because retained history had no SQL receipt cursor;
workspace restoration and terminal access remained blocked by that uncertainty.

The user requested immediate fixes and PR publication. This repair is independent
of PR #4434's macOS process identity and PR #4435's prepared prompt admission.

## Implementation

- Wait for generation-one authority during native resume as well as new sessions.
- Cancel only the failed processor's stream and report its original error promptly.
- Recover never-received, fully retained output through authenticated replay;
  preserve corruption, ownership, retention-gap, and no-resend safeguards.
- Prove the recovery sequence and existing workspace admission behavior with tests.

## Work orders

- [x] [Task 01: Restore resumed delivery](task-01-resumed-delivery.md)

## Validation and exclusions

Use lifecycle and agentctl race tests, relevant orchestrator recovery tests,
backend lint, specification/catalog validation, and normal commit hooks.
Do not mutate the user's live sessions, discard journals, replay prompts, or
clear recovery blocks without existing authoritative reconciliation.

## Results

Targeted race tests and lint pass. The optional full lifecycle suite has 15
failures that all reproduce on unchanged base a789b16d7; the work order records
the distinction and the read-only confirmation of retained recovery evidence.
