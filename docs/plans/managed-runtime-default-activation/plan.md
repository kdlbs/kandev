---
created: 2026-09-07
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-RUNTIME-UPDATES-002
system_design:
  - ../../specs/agents/system-design/runtime-default-activation.md
  - ../../specs/agents/system-design/runtime-updates-01.md
  - ../../specs/agents/system-design/runtime-updates-02.md
legacy_specs: []
---

# Implementation Plan: Managed Runtime Default Activation

## Overview

Kandev will activate a changed reviewed agent default during startup. The implementation first adds an idempotent selection-store reconciliation and then wires it before runtime consumers.

The existing Settings version action remains the downgrade path. Documentation and focused end-to-end evidence complete the change after the backend behavior is stable.

## Scope

### In scope

- Record the applied package and default version for each built-in managed agent.
- Remove an older operator selection when that agent's shipped generation changes.
- Preserve selections across restarts when the shipped generation is unchanged.
- Stop startup before readiness when reconciliation cannot finish.
- Keep active agent processes unchanged during backend recovery.
- Document default activation and the later downgrade path.

### Out of scope

- Replacing active agent processes during a Kandev upgrade.
- Querying npm or ACP-probing a reviewed default during startup.
- Changing native agents, passthrough commands, or authentication helpers.
- Resetting a selection when an unrelated Kandev release keeps the same agent default.
- Adding a new Settings control or API field.

## Technical approach

### Default-generation store

Add `managedruntime.DefaultGeneration` and `Store.ReconcileDefaults`. Each generation contains the built-in agent ID, trusted package, and exact default version.

Store the applied generation at `managed_runtime.default.<agent-id>`. Keep the existing active selection at `managed_runtime.active.<agent-id>`.

When the marker matches, preserve the selection. When the marker is absent or different, delete the selection before writing the current marker.

Use a stable agent order for deterministic behavior and logs. Return each read, delete, or save error to the startup caller.

### Startup integration

Build the generation list from registered agents that implement `agents.ManagedNPMRuntimeAgent`. Do not accept generation values from requests or stored selections.

Run reconciliation in `backendapp.provideServices` before agent discovery and runtime selection wiring. Return an initialization error before the backend reports readiness.

Keep current effective-version consumers unchanged. They will read no active selection after a generation reset and will use the embedded default.

### Documentation

Update the managed-agent how-to guide with the upgrade rule. Explain that the user can select an older stable version after startup.

Update internal bridge documentation and current Codex version claims. Make the displayed defaults match the embedded version catalog.

## Tests

- `AC-AGENTS-RUNTIME-UPDATES-002.1`, `.2`, `.4`, and `.7`: store tests cover changed, unchanged, and missing generation markers.
- `AC-AGENTS-RUNTIME-UPDATES-002.5`: lifecycle coverage proves that future resume commands use the reconciled effective version.
- `AC-AGENTS-RUNTIME-UPDATES-002.6`: startup composition tests cover every reconciliation error and readiness failure.
- `AC-AGENTS-RUNTIME-UPDATES-001.5` and `.6`: existing selection tests remain green with generation-aware precedence.

## E2E tests

The implementation does not change the Settings UI. The existing rollback scenario proves `AC-AGENTS-RUNTIME-UPDATES-002.3` after the backend activates a default.

Run the focused Playwright scenario in `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`. The test selects an older stable version and approves **Roll back runtime**.

## Work orders

- [x] [Task 01: Reconcile managed runtime generations](task-01-reconcile-default-generations.md)
- [x] [Task 02: Activate defaults during startup](task-02-activate-defaults-at-startup.md)
- [x] [Task 03: Document and prove upgrade behavior](task-03-document-and-prove-upgrade-behavior.md)

## Verification results

- `GOTMPDIR=... go test ./internal/agent/managedruntime -race -count=1` passed.
- The Task 02 focused backend command passed for backendapp, registry,
  hostutility, lifecycle, and Settings controller packages.
- `python3 scripts/lint-spec-files.py --all` passed.
- Public documentation tests passed: 61 tests; 46 published pages validated.
- The focused rollback E2E passed: 1 test in
  `tests/settings/agent-runtime-update.spec.ts`.
- `git diff --check` passed for the implementation, specifications, plans,
  decisions, public docs, and managed-runtime documentation.

## Risks

- The first release with this behavior removes all legacy managed-runtime selections because they have no generation marker.
- A user-selected version can be newer than the new Kandev default. The reset still uses the reviewed Kandev default by design.
- A partial multi-agent reset can occur before startup stops. The safe operation order makes the next startup complete the remaining resets.
- Backend recovery can reconnect to an existing process with its old runtime. Only its next process launch uses the new default.
