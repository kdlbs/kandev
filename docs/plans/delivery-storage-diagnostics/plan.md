---
created: 2026-10-27
updated: 2026-10-28
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
legacy_specs: []
---

# Implementation plan: Delivery storage diagnostics

## Overview

Show the actual storage cause and a useful recovery action in chat.
Start with source classification, then project observations, then complete desktop and phone presentation.
All work orders remain pending until a later implementation request.

The new requirement/design pair specializes diagnostics within Platform's existing delivery boundary.
The parent delivery requirement and design already approach their file size limits.
This package does not duplicate their journal, recovery, or admission requirements.

## Scope

### In scope

- Typed physical-capacity, quota, retention-limit, oversized-event, and unknown storage diagnostics.
- A preserved failure snapshot and a separate current observation.
- Bounded runtime status observation that works without another journal append or SQL write.
- An advisory at 90% filesystem usage, cleared below 85%, with shared 30-second capacity sampling.
- One shared chat notice with useful local or remote recovery guidance.
- Localized desktop and phone presentation, history, reload, and compatibility coverage.
- A concise public recovery section alongside the implemented behavior.

### Out of scope

- Changed disk reserves, capacity limits, event retention, cancellation, or prompt replay.
- Automatic cleanup, cleanup defaults, filesystem migration, or more storage permissions.
- User-configurable warning thresholds, incident aggregation, cross-task dashboards, or cache attribution.
- Production code or permanent tests during this design turn.

## Technical approach

### 1. Source diagnostic boundary

Use typed journal errors while retaining `errors.Is` compatibility.
Add the optional `streams.DeliveryStorageDiagnostic` contract to host-generated errors and authenticated status.
Extend API errors and `DeliveryHTTPError` without changing existing HTTP status or legacy codes.
Update the status client to retain capacity fields instead of discarding them.

### 2. Runtime and recovery projection

Add one bounded 30-second observation loop under lifecycle `Manager.Start`.
Capture current execution, native generation, and runtime lease identity before each request.
Reject replaced owners before updating the cache or publishing.
Agentctl measures its executor's journal filesystem, or its workspace filesystem when a journal is not configured.
Its shared sampler serves one capacity sample per proven environment/filesystem every 30 seconds.
The backend uses a read-only advisory route through the current executor client or provider proxy.
It never substitutes a backend filesystem measurement for local or remote agentctl evidence.
Keep requests bounded and merge samples only with verified source and filesystem identity.
Twenty local instances sharing one agentctl filesystem reuse one sample. Twenty separate Sprites require twenty independent source samples.
The 90% advisory leaves tasks running and clears below 85%.
The existing write-capacity guard remains independent and retains its current sampling behavior.

Use a runtime-only `RuntimeStorageHealthProvider` for session DTOs and boot state.
Publish the optional `task_session.runtime_storage_changed` event through session-authorized routing.
Reuse the cancellation snapshot provider pattern for bounded ownership and boot hydration.
Do not add a polling transcript, persistent health table, or per-browser poller.

For terminal errors, preserve evidence through `AgentEventPayload`, watcher events, and `AgentErrorCause.storage`.
Update normalization, equality, error publication, and task-summary copies as one projection contract.
Keep original failure evidence separate from later samples.

### 3. Chat presentation

Extend typed session-error decoding and recovery selection.
Create one shared storage view model and compact notice for task chat and Quick Chat.
Reuse `SessionRecoveryCard`, `SessionErrorDetails`, and existing recovery actions.
Use the current registered Storage settings route for eligible local failures.

The advisory is one padded row directly above each visible affected chat composer, below the transcript.
Extra guidance and measurements stay behind explicit details instead of occupying a permanent multi-line card.
Historical failures retain the existing recovery card and its stacked phone actions.
Keep the domain logic shared. Use phone/coarse-pointer sizing only for touch targets.
No dialog, drawer, new settings screen, or duplicate business logic is required.

### Compatibility matrix

| Provider or transport | Identity and capability | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Local/worktree, ACP or other native adapter | Current owned agentctl and advisory capability; journal optional | Correct local diagnosis and Storage settings entry | Source tests plus browser local flow |
| SSH/Docker/Kubernetes | Current owned instance and advisory capability | Executor-scoped diagnosis | Client/projection contract tests; no backend disk substitution |
| Sprites, including legacy delivery | Provider-verified Sprite VM/proxy and runtime storage capability | Per-Sprite workspace warning without a journal claim | Source/proxy fixtures and remote row rendering |
| Other capable executor | Version 1 advisory route only when supported | Same normalized diagnostics | Missing capability uses generic guidance |
| Older agentctl | Existing durable protocol without diagnostics | Existing recovery remains usable | Decoder fixture and legacy UI fallback |
| Current legacy delivery or passthrough | Runtime storage capability independent of durability | Valid advisory, with no journal claim | Unsupported older peers remain unavailable |
| Missing or foreign owner | Failed ownership or authentication | Ignore stale observations and retain recovery restrictions | Replacement/authentication tests |

## ASCII UI preview

Values below are placeholders, not measurements from an incident.
The control order, compact advisory placement, and historical phone action stacking are structural requirements.
Spacing and line breaks are illustrative.

### UI-01: Compact advisory, selected task chat or Quick Chat

```text
Desktop:
  Chat transcript ...
  ---------------------------------------------------------------
    ! Disk <usage>% used | <free> free       [Storage] [Details v]
  ---------------------------------------------------------------
  [ Write a message ...                                         ]

Phone:
  Chat transcript ...
  ---------------------------------------
    ! Disk <usage>% used  [Storage] [v]
  ---------------------------------------
  [ Write a message ...                 ]
```

This advisory starts at 90% usage. Tasks and the composer retain their normal state.
A fresh sample below 85% removes the advisory without changing session history.
Place the row immediately above the composer, with 8px top and bottom padding and content-sized height.
Phone controls retain 44px touch targets in the same row. Additional measurements move into Details when space is limited.
Remote rows name the relevant executor and use Details instead of the local Storage settings action.
For example: `! Sprite dev-12: disk <usage>% used  [Details v]`.
The separate write-capacity guard still uses "Host disk space is low" when safe writes become blocked.
Unknown values are omitted. After 90 seconds without a fresh sample, values say "Last reported".

### UI-02: Historical interruption, desktop recovery card

```text
Session interrupted by low disk space
Free space on this host, then resume this session.
At failure: <available> available; <required> required.
Reported: <failure time>
[Open storage settings] [Resume session]
[Technical details v]
```

The card owns the existing bounded vertical scroll region.
An existing pending or blocked recovery state retains its normal explanation and controls.
For remote storage, replace the host wording with the executor scope and omit the local settings action.

### UI-03: Historical interruption, phone recovery card

```text
Session interrupted by
low disk space

Free space on this host,
then resume this session.
Reported: <failure time>

[ Open storage settings ]
[ Resume session       ]
[ Technical details  v ]
```

Phone actions span the card and provide 44px or larger touch targets.
Details wrap in the card's single scroll region, without a nested code scroller.
UI-01 keeps its compact row on phones. Only the historical recovery card stacks actions.
Background observations preserve focus and transcript position.

These previews map to AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.1 through 003.10.
The advisory also maps to criteria 002.8 through 002.14.

## Tests

| Criteria | Targeted evidence |
| --- | --- |
| 001.1-001.4 | Journal/process/API/client `TestDeliveryStorage*` cases for each source reason and unknown measurements |
| 001.5-001.6 | Model, lifecycle, orchestrator, and task-summary `TestDeliveryStorage*` snapshot and sanitization cases |
| 002.1-002.14 | Source/lifecycle/metrics/DTO/gateway tests for hysteresis, executor isolation, shared samples, legacy Sprites, no remote wake, paused writes, and replaced owners |
| 003.1-003.5 | Shared view-model and recovery-card tests for copy, target scope, history, pending state, and copied details |
| 003.6-003.10 | Desktop/phone compact rows, advisory running state, locale changes, 44px targets, and catalog gates |

New tests use the `DeliveryStorage` name prefix where the work-order commands select that prefix.
Existing reserve and recovery tests remain part of the targeted regressions.

## E2E tests

Add `tests/session/delivery-storage-diagnostics.spec.ts` for `chromium`.
Add `tests/session/mobile-delivery-storage-diagnostics.spec.ts` for `mobile-chrome`.
Use `../../fixtures/test-base` and the existing session-error seeding helpers.

- Typed local failure survives reload and opens the correct Storage settings entry without an automatic cleanup request.
- The 90% advisory appears while work continues. An 88% sample retains it, and a sample below 85% clears it.
- Actual write pressure remains distinct and can render before a terminal event, without requiring another journal append.
- A historical failure retains its stamp and eligible Resume action after current capacity recovers.
- Remote and old-peer fixtures show supported guidance without invented local capacity or a misleading local cleanup action.
- Compact rows appear only above visible affected composers, with extra information collapsed. Multiple sessions do not stack multiple warning cards.
- Long details remain contained, touch actions meet 44px, and background updates preserve focus.
- Quick Chat uses the same normalized diagnosis and action eligibility.

Use causal event/request observers. Do not exhaust a real filesystem.
Backend integration tests own source-to-status and source-to-error proof.
Browser fixtures own visible actions, hydration, containment, and locale behavior.

## Work orders

- [x] [Task 01: Classify source storage diagnostics](task-01-source-diagnostics.md).
- [x] [Task 02: Project owned storage observations and failures](task-02-runtime-projection.md).
- [x] [Task 03: Show actionable desktop and phone recovery](task-03-recovery-ui.md).

Execute 01, then 02, then 03. No implementation delegation is assumed.

## Verification results

Design validation passed:

- `python3 scripts/list-docs.py validate`: 409 decisions and 1,769 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The repository documentation-coverage preflight accepted all three work orders and their requirement/design references.
  The actual design-only diff is exempt. A representative planned source change exercised package coverage without changing source.
- Local link and whitespace checks passed for all six new files. Catalog discovery includes the new requirement/design pair.
- `git diff --check -- docs/specs docs/plans/delivery-storage-diagnostics`: passed.

Product unit tests and browser checks remain pending implementation. Exact commands belong to each work order.

The October 28 refinement changes the advisory to 90% usage, below-85% clearance, and shared 30-second sampling.
Catalog validation, full specification lint, planned-source coverage, links, whitespace, and existing command-input checks passed again.
The later executor clarification moves sampling explicitly into agentctl, covers legacy-delivery Sprites, and uses one compact composer row.
The same design gates passed after that clarification. Product tests remain pending; no executor was started or changed for these checks.

## Implementation results

Implemented all three work orders:

- Journal storage errors carry closed reasons and bounded measurements while retaining `errors.Is` compatibility. Host errors expose the diagnostic through prompt admission, delivery HTTP errors, and the authenticated status client.
- Agentctl serves a read-only `GET /api/v1/runtime/storage` snapshot from a per-process shared 30-second disk capacity sampler. Status also carries it. The lifecycle health monitor observes it every 30 seconds, applies 90% warning and below-85% clearance with hysteresis, and publishes `task_session.runtime_storage_changed` over session-authorized routing. Session DTOs and boot payloads carry `runtime_storage_health`.
- Terminal evidence projects into `AgentErrorCause.storage` with `delivery_storage`, and chat renders the specific cause and eligible local Storage settings action. A compact advisory row sits above the composer, remote states name the executor, and details stay collapsed. All seven real locales and the pseudo locale include the new copy.
- Verification passed: backend build and full unit packages for journal, process, streams, metrics, lifecycle, models, and watcher; targeted race tests for source, sampler, client, lifecycle, watcher, models, and app wiring; frontend view-model, handler, notice, and recovery-copy tests; TypeScript typecheck; ESLint; `i18n:check`; public docs validation; changed-Go lint; and the managed desktop and phone E2E specs in `chromium` and `mobile-chrome`.

### Review remediation

Addressed all eight code-review findings:

- Browser epochs use a JavaScript-safe millisecond value, with cross-layer regression coverage.
- Owner-correlated write pressure and validated failure storage are projected through the SQL-independent live path. Healthy capacity samples do not clear blocked output, and pressure is visible below the usage threshold.
- Runtime storage revisions merge at the shared session merge and hydration boundary, so deferred HTTP, boot, subscribe, and state snapshots cannot replace newer live observations.
- Acceptance is fenced under the storage cache lock by current execution, native generation, and stream attachment. Replacement milestones retire predecessor entries.
- Ordinary status reads an already cached advisory snapshot with no filesystem I/O. The advisory sampler bounds filesystem identity lookup as well as capacity reads.
- Recovery copy distinguishes low byte capacity, allocation exhaustion, quota, journal limits, stream limits, oversized output, and generic write failures. Validated evidence is included in the copied technical details.
- Advisory and recovery controls use the shared fine-pointer desktop and coarse-pointer 44px sizing helper. The advisory stays on one row, truncates only the executor label, and keeps usage visible.
- Freshness uses backend receipt age plus browser-local monotonic elapsed time. Source observation time remains display-only and failed polls do not renew it.

Focused regression tests cover production epochs, quota pressure below 90% with paused output, late predecessor and attachment results, deferred snapshot ordering across epochs, bounded sampler identity/capacity deadlines, reason-specific copy and copied details, long executor labels, and stale receipt-age transitions.

## Risks

- A full disk can prevent persistence of its own error. Live status must remain independent of journal and SQL writes.
- A current healthy sample must not overwrite a recorded failure or resolve prompt uncertainty.
- Remote filesystem capacity must not use measurements from the backend host.
- Cause sanitizers and summary builders can silently drop new optional evidence.
- Other work can add a delivery health observer before implementation. Reuse its lifecycle owner and avoid duplicate warning loops.
- Reuse only measurements from the journal's actual filesystem. A configured status-bar path or remote root can identify a different mount.
- Sharing requires verified filesystem and source identity. Never share owner-specific pressure or use advisory cache data for write admission.
- Sprites currently omits a durable journal path. The advisory must work through its provider-verified proxy without requiring durable delivery or an agentctl token.
- Large active execution counts can delay polling. Keep concurrency bounded and record observation latency under the tested load.
- Public documentation must describe implemented controls only, so its update belongs to Task 03.
