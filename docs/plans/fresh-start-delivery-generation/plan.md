---
created: 2026-10-13
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Fix plan: fresh start under durable delivery

## Evidence and scope

The task "Suggest next prompt in chat input" (session `85ab3fe2`) is permanently
unable to start an agent.

1. On 2026-10-13 at 14:44, a build without the resumed-bootstrap fix
   (`a789b16d7`, before PR #4436) automatically resumed the session. That resume
   was its first durable generation. The backend dropped every event with
   "durable agent event processor is unavailable". After five minutes, the stall
   watchdog stopped the agent. That left the prompt `interrupted_unknown` and
   opened an `unknown_prompt_outcome` recovery block. PR #4436 fixed this
   initial failure.
2. Repeated Resume attempts failed with "agent delivery checkpoint missing".
   The user then chose Start fresh, which cleared `executors_running.resume_token`
   as AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2 requires.
3. On current main (`a02db6095`), Start fresh still fails. The executor assigns
   the launch harness generation 1 and stream `<incarnation>:g1`, because
   `applyDeliveryIdentity` advances the generation only for context
   continuation. The new execution therefore inherits the predecessor's
   retained, never-received stream. The resumed-bootstrap recovery requires a
   non-empty native session ID, so it is ineligible. The preflight then fails
   with "agent delivery checkpoint missing". Resume now fails the same way,
   because its only native identity source, the resume token, was cleared.

Persisted state (read-only):

| Record | State |
|---|---|
| `harness_session_generations` | One row: generation 1, native `60945aed`, `native_started` |
| `session_recovery_blocks` | Open `unknown_prompt_outcome`, consumer `agent_delivery`, generation 1 |
| `agent_delivery_submissions` | `prompt:5f17…`, `interrupted_unknown` |
| `agent_delivery_cursors`, `agent_delivery_inbox` | No rows |
| `executors_running.resume_token` | Empty |
| agentctl journal | Stream g1 retained from sequence 1, no acknowledgments |

The defect violates the design rule "A replacement native conversation creates a
new stream" in `durable-agent-delivery.md`. It also violates
AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2, because Start fresh can never
create the new provider-native session. No requirement changes. The stream
processing design now states the fresh-start generation rule explicitly.

## Implementation

- Carry an explicit "replace native conversation" intent from the orchestrator's
  `fresh_start` recovery through the executor and lifecycle launch requests to
  the execution and its ACP session-created event.
- In `applyDeliveryIdentity`, advance to generation N+1 and stream
  `<incarnation>:gN+1` for that intent whenever a committed generation exists.
  Without a committed generation, the intent is dropped and generation one is used.
- Relaunch a retained workspace-only or older-generation execution for the
  successor, as context continuation already does.
- Retire prior-generation unresolved journal submissions before the first
  prompt, as context continuation already does. Never dispatch them again.
- When the replacement's ACP session-created event arrives, commit generation
  N+1 with a predecessor compare-and-set and creation reason `native_replaced`.
  Replacement events wait for that commit through the existing bounded
  successor-generation wait. A failed or stale commit leaves them unadmitted.
- Leave the predecessor stream, submission, and block as history. Workspace and
  launch admission already read only the current generation.

## Work orders

- [x] [Task 01: Fresh start advances the delivery generation](task-01-fresh-start-generation.md)

## Validation and exclusions

Run the work order's targeted race tests, backend lint, `make fmt`, and the
specification and catalog validators.

Exclusions:

- Do not mutate the user's live session, journal, recovery block, or database.
- Do not change Resume's native-identity source.
- Do not resolve the predecessor block or resend the interrupted prompt.
- Do not change the UI. The existing Start fresh action becomes functional.

Recovery for the affected session after this fix ships: choose **Start fresh**.
Context continuation already advances the generation, and may work today, but
the UI does not offer it for this failure.

## Results

The author's original local verification is historical and has not been used
as delivery proof. Review added real-store schedules for execution rotation
after token acceptance, foreign identities, predecessor block retention, and
first-generation compatibility. Registered agentctl retirement and SQL-backed
successor waiting cover nondispatch, successful admission, cancellation, and
rejected commits. Hosted assertion and full-family verification gate delivery;
exact source, run, attempt and artifact receipts are retained outside the
repository by the delivery session.
