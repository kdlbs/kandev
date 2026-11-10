---
created: 2026-11-10
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
  - REQ-AGENTS-RUNTIME-MANAGER-002
  - REQ-AGENTS-RUNTIME-MANAGER-003
system_design:
  - ../../specs/agents/system-design/runtime-update-manager.md
legacy_specs: []
---

# Implementation plan: Agent runtime update manager

## Overview

Make runtime maintenance visible at the top of Settings > Agents.
Use one desktop dialog or phone drawer for the overview, individual version controls, policies, and bulk results.
First connect existing actions directly to shared details. Then add the overview and navigation. Finally add reviewed bulk updates.
Each work order leaves a usable flow and includes its own tests.

The user accepted the proposed desktop and phone sketches on 2026-11-10 and requested this design package.
All three work orders are implemented, with targeted unit, static, documentation, backend, and managed Playwright checks recorded below.

## Scope

### In scope

- The visible Runtime updates entry and available count.
- Direct per-agent version details and one overview-to-detail surface.
- Existing capability-specific recovery, manual guidance, policy drafts, and notification destinations.
- Update all and Update selected for eligible managed exact-version updates.
- Review preconditions on individual requests, partial results, retry, and interrupted-request recovery.
- Desktop, phone, narrow fine-pointer, and coarse-pointer coverage.
- Locale catalogs and the public runtime guide.

### Out of scope

- A new update mechanism, batch endpoint, persistent batch queue, or all-agent transaction.
- Bulk migration, native self-update, fallback updates, repair, or rollback.
- Changes to automatic-update defaults, release caching, notifications, external installations, running sessions, or executor ownership.
- Redesigning unrelated Agents toolbars, dynamic profiles, installation pages, or profile model settings.
- Dependency changes, a floating app-wide update indicator, and broad local verification or review passes.

## Inputs and settled choices

Agents owns the durable contract. UI supplies reusable Settings targets, save coordination, responsive primitives, and focus behavior.
No separate UI specification or new ADR is needed for this feature.
The [requirements](../../specs/agents/requirements/runtime-update-manager.md) contain the accepted outcomes.
The [system design](../../specs/agents/system-design/runtime-update-manager.md) defines capability and lifecycle boundaries.

The design adds two details to the sketches without changing their structure.
Dirty automatic-update policies receive explicit Save and Reset controls inside the overlay.
Bulk requests echo server review evidence to prevent another tab from changing the reviewed runtime before activation.

## Technical approach

### Task 01: Direct details

Extract reusable detail content from `AgentRuntimeUpdateControl` without replacing its version logic.
Move Agents-page open state above installed cards and pass explicit open callbacks to existing triggers and `AgentRuntimePolicies`.
Manage versions becomes a button that opens the selected runtime, including managed fallback details.
The bottom policy section remains usable until Task 02 replaces its presentation.

### Task 02: Manager and settings integration

Add `AgentRuntimeUpdateManager` and its domain hook above responsive branches in the Agents page.
Render the header action and the overview with current versions, available targets, ownership, and explicit unknown states.
Replace the bottom disclosure. Keep one mounted policy controller per runtime outside the conditional overlay.
Reuse `SettingsSaveCoordinator.saveAll`, domain Reset callbacks, and `runExclusive` for policy/update exclusion.

Extend `createSettingsTargetRegistry` and `SettingsTargetProvider` with action registrations.
Manager fragment targets activate views without a second hash listener or hidden DOM proxy.
Retain ordinary installed-card element targets and existing notification URLs.
The root manager preserves list position, selections, drafts, and focused identity across responsive changes.

### Task 03: Bulk admission and results

Add a pure capability/review selector and a store-owned bulk coordinator under `lib/agents/`.
Preview at most two rows concurrently. Freeze exact reviewed versions and selected identities before approval.
Submit individual approvals sequentially, without waiting for each server job to finish.
Observe matched job IDs through existing WS updates and bounded job-list recovery reads.
Closing or changing routes does not cancel approved work. Reload does not replay uncertain requests.

Extend existing preview and POST types with optional `reviewed_runtime` evidence.
Guarded requests require the same trusted managed identity, current selection, family/source, and update operation.
Compare evidence before policy mutation and at the guarded activation boundary.
Guarded jobs always prepare the captured managed candidate and cannot switch to native preparation.
Legacy individual requests retain their current contract. Servers without review evidence keep bulk disabled.

### Compatibility matrix

| Shape | Intended behavior | Evidence and unsupported fallback |
| --- | --- | --- |
| Managed npm candidate | Individual versions and reviewed bulk update | Existing runtime suites plus bulk tests. Missing identity disables bulk. |
| OpenCode managed family | Same-family bulk update, explicit individual migration | Existing migration coverage plus source/revision race tests. Native source is excluded. |
| Native host and managed fallback | Separate host observation and fallback detail | Fallback notification suites. Never count fallback maintenance as a host update. |
| Harness-owned updater | Existing target-free individual action | Self-update suites. Missing stable reference does not disable the individual updater. |
| External/manual | Ownership, unknown values, and verified guidance | Notification/identity suites. No guessed update request. |
| Disabled, unavailable, custom, unsupported | Explicit state and retained details | Late-target and unknown-state scenarios. No bulk mutation. |

## ASCII UI preview

The grouping, action order, one-overlay navigation, fixed regions, and explicit states are structural requirements.
Versions, counts, widths, icons, and spacing are illustrative. Product strings must use locale catalogs.
The nearest shipped mobile exemplar is `AgentRuntimeUpdateSurface`.
The curated picker-sheet pattern contributes Back navigation and fixed actions within one sheet.

### UI-00: Current entry, source verified

```text
Agents                                      [Install agents]
Installed agents
  Agent cards and profiles
  ... long page ...
> Agent runtime updates
  [Manage versions] -> scroll to installed card -> update trigger
```

### UI-01: Desktop page, ordinary entry

Entry: `/settings/agents`. Covers 001.1, 001.3, and 001.7.

```text
+------------------------------------------------------------------------+
| Agents                         [Runtime updates (4)] [Install agents]  |
| Install agents and manage profiles.                                    |
|------------------------------------------------------------------------|
| Installed agents                      [existing management controls]   |
| +--------------------------------------------------------------------+ |
| | Claude                                              [New profile] v | |
| | Runtime 0.100.0  [Update available]                                  | |
| | ... existing profiles ...                                          | |
| +--------------------------------------------------------------------+ |
| +--------------------------------------------------------------------+ |
| | Codex                                               [New profile] v | |
| | Runtime 2.5.0    [Update available]                                  | |
| +--------------------------------------------------------------------+ |
+------------------------------------------------------------------------+
```

The page owns scrolling. Runtime updates opens overview. A card update control opens that runtime's details.
No bottom runtime disclosure remains after Task 02.

### UI-02: Desktop overview, reviewed updates

Entry: header action or `#runtime-updates`. Covers 001.2 and 003.1 through 003.3.

```text
+------------------------------------------------------------------------+
| Runtime updates                                                    [X] |
| Updates apply to new sessions.                                         |
|------------------------------------------------------------------------|
| Available updates (4)                               [Check for updates] |
| [x] Claude       0.100.0 -> 0.103.0           [Update] [Manage versions] |
| [x] Codex        2.5.0   -> 2.6.0             [Update] [Manage versions] |
| [x] Codex server 0.166.0 -> 0.167.0           [Update] [Manage versions] |
| [x] OpenCode     2.2.5   -> 2.2.6             [Update] [Manage versions] |
|                                                                        |
| [Compatibility notice where returned by the preview]                   |
| > Up to date                                                           |
| > Unknown checks                                                      |
| > Managed externally                                                  |
| > Unavailable registrations                                           |
|------------------------------------------------------------------------|
| Manual updates turn off automatic updates for the affected runtimes.    |
| [Close]                                             [Update all (4)]   |
+------------------------------------------------------------------------+
```

Header and footer stay fixed. The list is the only scrolling region.
Checkboxes and bulk footer arrive in Task 03. Task 02 retains individual actions without placeholder bulk controls.
Individual-only availability can exceed the bulk count. The overview explains that difference.

### UI-03: Runtime detail and dirty policy

Entry: Manage versions, installed-card trigger, or `#runtime-update-claude-acp`.
Covers 001.3 through 001.5, 002.1, and 003.6.

```text
+------------------------------------------------------------------------+
| [< All runtimes]                    Claude                         [X] |
|------------------------------------------------------------------------|
| Current runtime   0.100.0                                              |
| Latest stable     0.103.0                                              |
| Version           [0.103.0 (latest)                                  v] |
| [Use Kandev default]                                                   |
| Automatic updates                                             [ Off ] |
| > Runtime source and technical details                                 |
| [Retained outcome, job output, or required compatibility notice]        |
|------------------------------------------------------------------------|
| Unsaved policy changes                 [Reset] [Save changes]          |
| Save or reset this policy before updating this runtime.                |
| [Cancel]                                     [Update Claude: disabled] |
+------------------------------------------------------------------------+
```

The dirty row disappears after Save or Reset. The version action uses the existing operation label.
An older version offers Roll back. Harness details omit the version picker. External details offer guidance.
Back retains overview state. Task 01 uses the same detail region without All runtimes until overview exists.

### UI-04: Phone page and overview drawer

Entry: visible header action or notification link. Covers 001.1, 001.6, 001.7, and 002.3.

```text
+------------------------------------+
| Agents            [Install agents] |
| [       Runtime updates (4)      ] |
| Installed agents                   |
| ... existing cards and profiles ...|
+------------------------------------+

+------------------------------------+
|                ----                |
| Runtime updates                [X] |
| Applies to new sessions.           |
|------------------------------------|
| Available updates (4)              |
| [x] Claude                         |
|     0.100.0 -> 0.103.0             |
|     [Update] [Manage versions]     |
|------------------------------------|
| [x] Codex                          |
|     2.5.0 -> 2.6.0                 |
|     [Update] [Manage versions]     |
| ... one scrolling body ...         |
|------------------------------------|
| [        Update all (4)         ] |
|            safe area               |
+------------------------------------+
```

The tall bottom drawer fits `dvh`. Header/footer remain fixed with safe-area clearance.
Actual touch targets are at least 44px. The primary phone action uses a nominal 48px height.
Details replace the list within this drawer and show Back, rather than opening another drawer.
Policy Save and Reset share the footer while dirty.

### UI-05: Loading, unknown, and retry

Covers 001.2 and 002.4. The same states use the one-column phone body.

```text
Runtime updates
  Checking runtime versions...                    [approval disabled]

Runtime updates
  Could not refresh. Last known information remains visible. [Retry]
  Claude   Current: Unknown   Latest: 0.103.0    [Manage versions]
  Cursor   Managed externally                   [Manual guidance]
  Selected target needs review.                 [Retry preview]
```

No-update entry shows an explicit empty available group and keeps version management accessible.
Unknown checks never display an Up to date claim or enter bulk selection.

### UI-06: Bulk progress and partial result

Covers 003.3 through 003.5 on desktop and phone.

```text
Runtime updates
  Claude       0.100.0 -> 0.103.0   Updated
  Codex        2.5.0   -> 2.6.0     Validating candidate...
  Codex server 0.166.0 -> 0.167.0   Failed: candidate could not start
                                         [Details] [Review retry]
  OpenCode     2.2.5   -> 2.2.6     Submitting...
------------------------------------------------------------------
  1 updated, 1 failed, 2 in progress.                  [Close]

After completion:
  3 updated, 1 failed.                         [Retry failed (1)]

After an unconfirmed request:
  Result not confirmed.                         [Refresh status]
  Review this runtime before another update.
```

Queued and accepted states are progress. They never count as success.
Retry returns only failed rows to review. Successful rows retain their results.

## Tests

The following test names are implemented additions or extensions. Every requirement number in this section has prefix `AC-AGENTS-RUNTIME-MANAGER-`.

| Criteria | Unit/integration file and named scenario |
| --- | --- |
| 001.1, 001.2, 002.4 | `lib/agents/runtime-update-manager.test.ts`: available count excludes unknown, external, unavailable, and busy rows |
| 001.3, 001.4 | `components/settings/agent-runtime-update-control.test.tsx`: external opener selects one runtime without a card jump |
| 001.4 | Existing Copilot, self-update, fallback, operation, and runtime-identity tests retain their mode-specific assertions |
| 001.5, 003.6 | `components/settings/agent-runtime-update-manager.test.tsx`: dirty policy saves explicitly, survives dismissal, and blocks conflicting admission |
| 001.7, 002.1, 002.2 | `lib/settings-discovery/target.test.ts` and `settings-target-provider.test.tsx`: action targets handle initial, repeated, and late requests without changing element targets |
| 002.3, 002.4 | `agent-runtime-update-manager.test.tsx`: back navigation and breakpoint changes preserve state, stale previews cannot replace newer detail |
| 003.1, 003.2 | `lib/agents/runtime-update-manager.test.ts`: exact review excludes unsupported operations and preserves deselection |
| 003.3, 003.6 | `agent_update_review_test.go`: changed identity or source rejects before policy writes and guarded activation preserves newer selection |
| 003.3 | `agent_update_handlers_test.go` and `agent-update-api.test.ts`: review evidence round trip, incompatible forms, legacy request compatibility |
| 003.3, 003.4 | `lib/agents/runtime-update-batch.test.ts`: one submission per target, partial failure, matching job IDs, failed-only retry |
| 003.4, 003.5 | `runtime-update-batch.test.ts`: dismissing caller does not cancel work, lost response stays unconfirmed, recovery rejects older snapshots |

Backend tests use fake updaters and temporary selections. No global CLI or real npm installation is modified.
Cancellation coverage includes one observer unmounting during a shared recovery read while another observer completes.

## E2E tests

Task 01 extends the existing update and notification suites for direct opening without an anchor jump.
Task 02 updates the compact-settings helper to manager entry and preserves every existing semantic scenario.
Task 03 adds `agent-runtime-bulk-update.spec.ts` and `mobile-agent-runtime-bulk-update.spec.ts` with shared fixture helpers.

| Flow | Criteria | Playwright files/projects |
| --- | --- | --- |
| Header entry, direct details, initial/same-route/late/inactive targets | 001.1-001.3, 002.1, 002.2 | Existing `agent-runtime-notifications.spec.ts` / `chromium` and mobile counterpart / `mobile-chrome` |
| Saved policy, failed save, draft retained after close, clean/dirty refresh | 001.5, 002.3, 003.6 | Existing notification suites plus focused manager scenarios |
| Pinned update, rollback/default, self-update, Copilot notice, fallback identity | 001.4 | Existing runtime-update, self-update, Copilot, and identity suites on both projects |
| Containment, single scroll owner, footer reachability, focus return | 001.6, 001.7 | Phone notification suite, 767/768px desktop cases, and coarse-pointer tablet case |
| Review all/subset/none, exclude individual-only rows, preserved deselection | 003.1, 003.2 | New bulk desktop and phone specs |
| Partial failure, duplicate click, stale review, different active job, retry only failed | 003.3, 003.4 | New bulk desktop and phone specs |
| Close/reopen, route change, lost response, reload/reconnect recovery without replay | 002.3, 003.5 | New bulk specs with request counters and retained job fixtures |

Use `../../fixtures/test-base` in nested settings specs and existing runtime fixture patterns.
Use real visible openers and causal HTTP/WS waits. Do not replace outcomes with visibility-only assertions.
Measure phone hit targets, viewport containment, zero document overflow, and the actual desktop control size.
Compare focused desktop and phone screenshots with UI-01 through UI-06 during each work order's browser checks.

## Work orders

1. [x] [Task 01: Open version details directly](task-01-direct-runtime-details.md) (`done`, wave 1, no dependencies).
2. [x] [Task 02: Add the responsive update manager](task-02-runtime-update-manager.md) (`done`, wave 2, depends on Task 01).
3. [x] [Task 03: Add reviewed bulk updates](task-03-reviewed-bulk-updates.md) (`done`, wave 3, depends on Task 02).

Execute sequentially. No work order is parallel-safe because they share the page, update controller, API, fixtures, and locale catalogs.

## Verification

Each work order contains its exact targeted commands. All blocks start from the repository root and use isolated directory changes.
Before the first package command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` once.
The managed E2E runner rebuilds production web/backend assets. Desktop and phone runs must not overlap.
Run the listed targeted checks rather than adding generic QA, review, or full-suite work.

Design-package gates:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
git status --short -- docs/specs docs/plans
```

The PR-documentation `validateCoverage` preflight must resolve every work order, criterion, requirement, and design in this package.
Run it with the planned product paths as coverage triggers. Documentation-only exemption alone does not establish traceability.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/agent-runtime-update-manager/';
const orders = fs.readdirSync(root).filter(name => /^task-\d+.*\.md$/.test(name)).map(name => root + name);
const paths = [root + 'plan.md', ...orders,
  'docs/specs/agents/requirements/runtime-update-manager.md',
  'docs/specs/agents/system-design/runtime-update-manager.md'];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const triggers = [
  'apps/web/components/settings/agent-runtime-update-control.tsx',
  'apps/web/app/settings/agents/page.tsx',
  'apps/backend/internal/agent/settings/controller/agent_update.go'];
for (let index = 0; index < orders.length; index++) {
  const result = validateCoverage({fileContents, changedFiles: [
    {filename: triggers[index], status: 'modified'},
    {filename: orders[index], status: 'added'}]});
  console.log(orders[index], result.status, result.errors);
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Review remediation

Static review identified nine correctness gaps. All are fixed with focused regressions:

- Bulk coordinator runs and retained-job recovery capture an authenticated authority fingerprint, stop unsubmitted rows on identity or permission loss, never publish a replaced identity's responses, and classify 401/403 as authority failures. Malformed successful POST responses remain unconfirmed.
- Overview Update and Review update actions open the normal detail review; a failed or changed preview cannot be bypassed by the row action.
- Retry reloads current status and exact previews for failed rows, preserves successful rows, and replaces rejected evidence before another admission.
- A completed batch exposes Review updates, which starts a fresh review without a route remount or reload.
- Bulk approval is disabled by loading, failed refresh, or no currently eligible selected row, and the coordinator enforces the eligible-key set at admission.
- Retained-job recovery reacts to WebSocket reconnect as well as browser online and manager entry.
- Unsupported and unavailable rows always expose read-only Details, including retained outcomes without a guidance URL.
- Overview scroll position and expanded status groups survive overview-to-detail Back navigation.
- Policy details share the single scroll body inside a flex-column detail surface, so short viewports keep fixed actions reachable.
- Bulk admission surfaces the shared busy state when exclusive work refuses a start, and rejected refresh reads become explicit failures.
- Review rows label the observed current version, and automatic-update help ids are scoped to their switch and omitted when no help renders.

## Verification results

Implementation completed on 2026-11-10 and review remediation completed the same day.

- Backend: `go test ./internal/agent/settings/... -count=1` passed for controller, handlers, dto, store, catalog, cliflags, and profileconfig; `go test -race ./internal/agent/settings/controller -run 'TestReviewedAgentUpdate' -count=1` passed; `bash scripts/lint-go-changed ...` reported 0 issues.
- Web unit suites: combined Task 01 and Task 02 focused Vitest run passed 116 tests across 15 files; Task 03 focused run passed 68 tests across 11 files, including authority fencing, malformed 202, fresh retry evidence, completion reset, WebSocket reconnect recovery, refresh gating, read-only details, and scroll restoration.
- Web static checks: `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed. Changed-file Oxlint reported no warnings.
- Documentation: `node --test scripts/validate-public-docs.test.mjs` passed 63 tests; `node scripts/validate-public-docs.mjs` validated 55 published pages.
- Specification gates: `python3 scripts/list-docs.py validate` validated 446 decisions and 1,965 specifications; `python3 scripts/lint-spec-files.test.py` passed 36 tests; `python3 scripts/lint-spec-files.py --all` passed; `git diff --check` passed.
- Bulk E2E: `agent-runtime-bulk-update.spec.ts` passed 2/2 and `mobile-agent-runtime-bulk-update.spec.ts` passed 2/2 on their Playwright projects.
- Runtime E2E: desktop notifications passed 7/7; phone notifications passed 4/4; the desktop runtime update, self-update, Copilot, identity, and profile-discovery group passed 27/27; the phone group passed 20/20 after the scroll-owner assertion was corrected.
- Managed Playwright runs rebuilt production web/backend assets through the repository runner. After review remediation, the desktop notifications suite (7/7), phone notifications suite (4/4), and both bulk suites (2/2 each) were rerun and passed.

New requirement and design documents are active. All work orders are done.

## Risks

- A controlled wrapper must not reset detail state on every parent render or mount duplicate update portals.
- A modal must not strand policy Save behind its focus boundary or unregister drafts when closed.
- Shared target actions must preserve existing element targets and delayed registration behavior.
- A header availability count can differ from the bulk count because some updates require individual actions.
- A client-only preview check cannot close admission races. Bulk requires server review evidence and a guarded managed preparation path.
- Existing jobs are retained per runtime, not as a durable aggregate batch. Missing evidence must remain unconfirmed after reload.
- Companion tests currently assert the bottom disclosure. Their behavior assertions must migrate together without losing notification or policy coverage.

## Companion packages and documentation

The original [notification package](../agent-runtime-notifications/plan.md),
[compact settings package](../agent-runtime-settings-compact/plan.md),
[summary package](../agent-runtime-update-summary/plan.md), and
[indicator removal](../remove-agent-runtime-update-indicator/plan.md) retain their historical statuses and results.
This package replaces their settings composition and destination assertions only.
It preserves the absent app-wide floating indicator, release preferences, summary timing, and notification deduplication.
Their supersession notes identify this package without rewriting historical evidence.

Implementation updated `docs/public/agents-and-profiles.md` for the new entry, direct version flow, Save behavior, and bulk scope.
The root README and screenshot guide contain no bottom-disclosure instructions that need rewriting.

The replaced placement criteria in the notification and summary contracts now link to the manager contract.
The manager requirement is active, this design is current, and this plan is implemented.
Each work order records its actual command results and final acceptance status.
