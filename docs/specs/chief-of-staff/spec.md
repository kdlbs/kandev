---
status: implemented
created: 2026-09-07
---

# Configurable orchestration personas

One native Kandev persona is the user's conversational coordinator. It delegates
implementation to workers selected by agent and executor profile, retains compact
management context, and reacts to manual messages and autonomous workflow events.
OpenClaw and Hermes are not dependencies. Personal-account coordination of work
agents is allowed; substantive work must use its selected worker account.

## Acceptance

- Users can create a Chief of staff persona, edit its instructions and permissions,
  choose its execution profile, and return to its conversation.
- Manual messages and scheduled routines enter the existing durable Office run
  pipeline. Worker completion and blockers reach the coordinator through existing
  task relationships and wake events.
- The coordinator delegates rather than implementing, reports task identifiers,
  and requests detailed worker context only when needed.
- Context policy must have executable limits, not only prompting. Conversation
  history remains available separately from model session context.
- Worker execution credentials remain bound to configured agent/executor profiles.
- Restart/retry must preserve tasks and avoid duplicate dispatch. Paused personas
  do not execute autonomous work. Failures must be visible.

## Implementation boundaries

Reuse Office agents, instructions, memory, routines, permissions and task comments.
Prefer native web conversation for the first working installation. External chat
platforms and full host-outage recovery are separate from the native interface.

## Validation

Backend behavior tests, frontend configuration tests, and a local end-to-end
manual-message -> dispatch -> worker result -> coordinator response demonstration
are required before declaring the feature complete.

Implementation and pipeline validation completed on 2026-09-07. See
[verification record](../../plans/chief-of-staff/plan.md). Real provider decision
quality and subscription identity remain deployment validation, not mock-test claims.
