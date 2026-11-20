# Lifecycle evidence

Use this checklist for changes to scheduling, background delivery, or task/session lifecycle.
Skip it for changes that do not affect those behaviors.

Before implementation, assess these scenarios:

- Pause and resume: does accepted work remain durable, or can the contract discard it?
- Restart and recovery: can fresh services recover the same persisted work?
- Duplicate and stale delivery: can replay duplicate an effect or let an old owner change successor work?
- Ownership changes: does reassignment, cancellation, or replacement preserve the contract?
- Concurrent or merged work: does active work retain required data and effects?
- Entry paths: do API, UI, runtime, and event paths exercise the same contract?

For each applicable scenario, record its owning acceptance criterion, observable invariant, regression test, exact command, and result in the current work order. Give a concrete reason for each exclusion. A broad passing suite does not replace a scenario assertion.

If evidence is missing or blocked, name the gap in the completion report. Do not claim complete lifecycle verification while a required scenario lacks proof.

During the Test phase, give the user the relevant reproduction steps for an isolated secondary instance. Use the existing `scripts/dev-isolated` procedure. Keep the workflow's manual phase transitions.

Keep this file as the single lifecycle checklist. Do not copy it into another ledger or add a generic review gate.
