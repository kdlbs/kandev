---
created: 2026-10-04
status: completed
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-003
system_design:
  - ../../specs/agents/system-design/runtime-update-summary.md
legacy_specs: []
---

# Implementation plan: Combine agent runtime update notifications

## Overview

Deliver one delayed availability summary across notification providers, with individual runtime details in Settings.
One sequential work order covers the collector, delivery, payload, and rendered flow because these contracts must change together.
The user requested grouping for startup. The fixed 30-second window is the selected implementation detail; it also handles periodic bursts.

Agents owns this capability because runtime discovery and version identity define summary membership.
This package extends existing runtime awareness; completed companion plans retain their historical results.

## Scope

### In scope

- AC-AGENTS-RUNTIME-NOTIFY-003.1 through 003.9.
- A controller-owned collector, preference-aware grouped delivery, and compatible structured payloads.
- Local, browser/native, system, and external-provider summary copy.
- Desktop/phone navigation, shipped translations, and public how-to guidance.

### Out of scope

Generic grouping configuration, notification inboxes, modal deferral, runtime activation changes, feature flags, schema migrations, and persistent badges.

## Technical approach

Extend `runtime_update_background.go` and `runtime_update_notifications.go` with a fixed-window collector under the existing background lifecycle.
Keep batch delivery in `notifications/service/runtime_updates.go` and reuse child `InsertDelivery`/`DeleteDelivery` claims.
Update `providers.Message`, local WS payloads, frontend notification types, queue consumption, and the toast bridge in one pass.
Use a typed notification member list and keep singular payload compatibility.
Follow the [system design](../../specs/agents/system-design/runtime-update-summary.md) for revalidation, claims, and cancellation.

| Channel                 | Identity and behavior                                     | Evidence / fallback                                             |
| ----------------------- | --------------------------------------------------------- | --------------------------------------------------------------- |
| Local WS                | User-scoped message; one summary with typed members       | Provider tests; disconnected subscriber releases child claims   |
| Browser/native client   | Summary from local delivery; existing transport selection | Toast hook test captures one native/browser send                |
| Backend system          | Existing title/body send; one message per provider        | Capturing provider test; existing unavailable-provider behavior |
| External provider       | Existing title/body interface; one summary                | Capturing provider test; subscriptions remain authoritative     |
| Legacy singular/release | Existing identity and action                              | Existing handler/toast tests; no generic grouping               |

## ASCII UI preview

### UI-01: App route after startup checks

Entry: Any app route. State: Four available updates within the collection window.

```text
Before:                       After the fixed collection window:
[Copilot runtime update]      +----------------------------------+
[Gemini runtime update]       | 4 agent runtime updates available |
[Codex runtime update]        | Review versions in Settings.     |
[Claude runtime update]       | [Review updates]                 |
                             +----------------------------------+
```

During collection, the app shows no availability toast. An empty group shows none.
One eligible runtime uses the existing named toast after the same delay.
Failed/interrupted outcomes appear individually without this delay.

Desktop and phone share this composition. Phone uses one compact viewport-contained notice with a touch-sized action.
The action navigates directly to the existing Settings page and expands its runtime section.
Settings retains one page scroll owner, all per-runtime details, and existing version-selection drawers.
No new drawer, hover interaction, or persistent indicator is required.
Structural requirements: one notice, one explicit action, count, and direct navigation.
Copy and spacing are illustrative; render localized copy with existing primitives.
UI-01 maps to AC-AGENTS-RUNTIME-NOTIFY-003.2, 003.3, 003.4, and 003.8.

## Tests

- Controller `runtime_update_notifications_test.go` and proposed `runtime_update_summary_test.go`: fixed timer, overlapping passes/replays, identity replacement, revalidation, cancellation, immediate outcomes (003.1, 003.2, 003.6, 003.7, 003.9).
- Notification service proposed `runtime_update_summary_test.go`: recipient isolation, per-provider member claims, membership changes, muted providers, partial claim failure, send failure, replay, and singular fallback (003.2-003.5, 003.9).
- Provider `local_test.go`: summary members survive WS serialization and disconnected delivery returns an error (003.3, 003.9).
- Web `use-update-available-toast.test.ts`, `update-available-toast-bridge.test.tsx`, `notifications.test.ts`, and `ui-slice.test.ts`: localized count/action, one toast and native/browser send, compatible scalar notices, summary-triggered refresh, queue identity (003.3, 003.4, 003.7, 003.8).

## E2E tests

Extend existing `agent-runtime-notifications-helpers.ts` and the desktop/phone specs.
Retain saved-consent, ownership, and version-dialog scenarios from the existing helper.
Add a real backend startup/reconnect batch scenario proving no early availability notice, one final summary, and an expanded Settings destination. Keep the injected-summary scenario for presentation coverage.
Do not replace backend grouping evidence with injected final summary payloads alone.
Use backend unit fake clocks for timing boundaries; use causal WS waits for final browser delivery.
A bounded negative assertion can use the sanctioned `dwell` helper with its reason.
Add replay/reload coverage and an outcome delivered while availability collection remains pending.
Assert phone target dimensions, viewport containment, and no page horizontal overflow.
Capture one desktop and phone summary state and compare it with UI-01.

## Work orders

- [x] [Task 01: Deliver grouped runtime availability](task-01-group-runtime-availability.md) (done; wave 1; no dependencies).

## Verification results

Design validation on 2026-10-04:

- `python3 scripts/list-docs.py validate`: passed (348 decisions, 1338 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR documentation `validateCoverage` preflight: docs-only diff is exempt. Simulated planned runtime change validates this work order and all linked references as covered.
- `git diff --check`: passed at the design handoff.

Implementation validation on 2026-10-04:

- Backend controller, notification service, and provider packages passed the race-enabled tests. Both new summary test suites are included in those package runs.
- The backend build and Vite production build passed. Vite reported its existing chunk-size, dynamic-import, and advancedChunks warnings; cross-platform helper builds reported that codesign tools were unavailable for the Darwin artifacts.
- Focused frontend tests passed (76 tests across four files), as did web typecheck, Traditional Chinese generation, pseudo-locale generation, i18n checks, and the new-code i18n ratchet.
- Desktop runtime-notification E2E passed all five cases; mobile Chrome passed both cases. Focused capture runs produced desktop and phone summary frames. Both were visually checked against UI-01: one viewport-contained notice, direct Settings action, and a touch-sized phone action.
- Public documentation validation passed (62 validator tests and 47 published pages). Specification validation passed (348 decisions and 1338 specifications), and all specification files passed lint.
- `git diff --check` passed after the initial implementation.

Review remediation validation on 2026-10-04:

- The controller race suite passed with independent terminal outcome delivery, coalesced availability admission, revalidated current-version metadata, and preservation of simultaneous availability/outcome observations.
- Focused backendapp E2E-hook tests and `make -C apps/backend build` passed. The build reported unavailable Darwin codesign tools for helper artifacts.
- Desktop runtime-notification E2E passed 6/6 cases; mobile Chrome passed 3/3. The added real backend flow covers reconnect replay, a terminal outcome during the pending availability window, the delayed grouped summary, direct Settings navigation, and per-member deduplication after reload. Existing injected-summary coverage remains.
- After the final change preserving availability when a status also carries a terminal outcome, the real backend startup/reconnect test passed against the final backend build on Chromium (1/1) and mobile Chrome (1/1).
- `git diff --check` passed after the initial review remediation.

Additional PR-review remediation on 2026-10-05:

- Summary revalidation and provider delivery now run on one lifecycle-owned worker. The collector continues to process observations while provider sends are blocked; a bounded queue coalesces one pending notice per trusted runtime, and shutdown cancels active sends and clears pending work.
- Removing the last eligible member preserves the original 30-second deadline. A barrier regression proves a later group reaches the bounded delivery queue while the first provider send is blocked, and a terminal outcome is delivered before the first send is released.
- The system provider localizes grouped OS copy from its configured locale or the host locale. Its tests cover every shipped language and English fallback.
- The backend readiness test now synchronizes with `testing/synctest`. The manual-runtime background test starts the actual collector and verifies the delayed Kimi member, versions, and Settings URL; the WebSocket payload fixture contains both members matching its count.
- The real startup/reconnect E2E records its conservative early-window timestamp before restart, arms the terminal-outcome wait after reconnect, and allows reload overhead in the replay timeout. Presentation injection coverage remains in the existing suite.
- Controller, notification service, and provider race tests passed. Backend E2E-hook tests, backend build and lint passed. Focused notification Vitest tests passed (76/76), web typecheck passed, and focused ESLint/Prettier passed.
- Desktop runtime-notification E2E passed 6/6 cases; mobile Chrome passed 3/3. The first result used the real backend startup/reconnect route and replay-after-reload flow.
- `git diff --check` passed.

## Risks

- Provider-specific prior claims can reduce each summary to a different member set; calculate its count after claims.
- Source checks can finish after the fixed window; revalidation adds bounded lookup time and later arrivals use another window.
- Existing native/system channels can both be enabled. Grouping does not introduce cross-channel exactly-once delivery.
- Preserve the existing claim-before-send crash limitation and document rollback failures.
- Restore quiesce and reconnect races require cancellation tests, not only timer tests.
