---
created: 2026-10-10
status: implemented
requirements:
  - REQ-WORKSPACES-DELETION-002
system_design:
  - ../../specs/workspaces/system-design/deletion.md
legacy_specs: []
---

# Implementation plan: Workspace deletion catalogue preservation

## Overview

An accepted Settings workspace deletion currently publishes a filtered array
captured before the request. Independent live creation disappears and an
unrelated live deletion is resurrected at the diagnostic baseline. One completed
sequential work order replaces publication with a current-owning-store target filter, proved by an
independently authored real-page regression and compatibility controls.

Workspaces owns the failed lifecycle/catalogue contract. Extend the existing
[deletion requirement](../../specs/workspaces/requirements/deletion.md) with
requirement 002 and its [paired design](../../specs/workspaces/system-design/deletion.md).
Requirement 001 stays unchanged, including its Office/native-home destination.
The shared Settings editor keeps its actual `/settings/workspaces` return.
The [settings update pair](../../specs/workspaces/system-design/workspace-settings-updates.md)
owns Save/Add Workspace and treats delete as an independent boundary.

## Scope

### In scope

- Accepted-list publication in `useWorkspaceDeleteDraft` and its immediate
  `useWorkspaceEditForm` glue, in `workspace-edit-client.tsx` only.
- Current membership, relative order, survivor descriptors and current active
  identity/revision under the existing `setWorkspaces` policy.
- Real Page/provider/request/registered-notification/picker causal evidence,
  ordinary accepted and rejected controls, and exact task-defined checks.
- Four internal documentation artifacts, uncommitted at this design handoff.

### Out of scope

- Backend/Office deletion or cascade redesign; route or selection-policy changes.
- Global catalogue arbitration, other writers, stale draft saves, lifecycle
  hardening, configuration, feature flags, cosmetics or new product copy.
- Product builds, browser/database runs or broad backend/frontend/E2E replays
  for unchanged markup. Delegates, recursive tasks, sessions and model switches.

## Technical approach

Supply the existing form `useAppStoreApi` to `useWorkspaceDeleteDraft` instead
of the captured array/setter. After successful `deleteRequest.run`, synchronously
read `storeApi.getState()`, filter current items by target identity, and invoke
that current state's `setWorkspaces`. Remove the unused form subscriptions.
No new helper/module, store action, array reconstruction, selection action or
request is needed. Preserve every survivor as currently represented.

Preserve the exact confirmation guard, owner/manage card gate,
`officeEnabled` request choice, payload, error toast, dialog/name behavior,
and `runWithNavigationBlockerBypassed` around the existing Settings return.
Keep merged Save/Add Workspace source and tests unchanged. Read the
implementation-release head before editing; a required new boundary or rendered
change must checkpoint ROOT.

## Tests

Independently author
`apps/web/app/settings/workspace/workspace-delete-catalogue.integration.test.tsx`.
An optional new `workspace-delete-catalogue.test-helpers.tsx` owns fixture size.
Use the real Page, StateProvider/createAppStore, SettingsSaveProvider, normal
client routing, Toast/Tooltip providers, useRequest/action/fetchJson, registered
workspace handlers, and actual AppSidebarWorkspacePicker. Isolate only external
fetch/WS delivery; reject unexpected requests. Observe real store and rendered
choices before and after settlement. Do not read, copy, import or replay ROOT's
protected candidate source.

| Criteria | Independently authored case | Evidence |
| --- | --- | --- |
| .1, .2 | `preservesMixedLiveMembershipAfterAcceptedDelete` | Hold target DELETE; real created row is selectable and unrelated deleted row absent before ACK; 204 removes only target; normal Settings return retains those choices and relative order |
| .1, .2 | `preservesSurvivorDescriptorsAfterAcceptedDelete` | Real independent update while pending; after ACK retain current picker name and explicitly changed metadata/defaults/idle/timestamps, not fixture-derived expected output |
| .3 | `preservesSelectionChangedDuringDelete` | Real `setActiveWorkspace` while pending; acceptance retains current activeId/revision and catalogue |
| .3 | `retainsNullIdentityFallback` | Real null selection while pending; ACK selects first current survivor using existing setter and keeps the already-current revision |
| .1, .3 | `preservesNotifiedTargetRemovalAndFallback` | Real target `workspace.deleted` before ACK; target stays absent and existing event-selected activeId/revision retained |
| .2, .4, .5 | `acceptsOrdinarySettingsDeletion` | Both office feature values use the real expected DELETE URL, exact confirm_name, 204, only target removal and unchanged Settings return |
| .1, .3, .4 | `rejectedDeletePreservesLiveCatalogueAndConfirmation` | Hold DELETE, apply independent changes, reject 503; real toast, current catalogue/selection, dialog/name and source route retained; no request-owned removal |
| .5 | `mismatchedConfirmationDoesNotDelete` | Wrong/case-different name keeps actual confirm disabled and sends no DELETE; exact name permits normal action |
| .5 | `hidesDeleteWithoutManageScope` | Real form without manage scope exposes no delete card or request |
| .3, .4 | `acceptsLastWorkspaceDeletionWithoutNewSelectionPolicy` | Empty survivors; target removal and Settings return retain the observed slice policy, no invented Office setup redirect or selection mutation |

The mixed-membership case must reach a causal post-ACK assertion failure on
unmodified production before the correction. The survivor update case must
independently demonstrate stale descriptor loss. Ordinary accepted and rejected
controls must pass on that same RED run. Setup exceptions, unhandled async
errors, or transport mismatches are not RED evidence. Assert explicit values
and raw catalogue order; picker presentation order may differ legitimately.
Own and settle deferred responses/read effects and restore providers, globals
and route state. Retain every original command handle to actual completion.

## End-to-end and mobile evidence

The bounded end-to-end proof runs actual controls through real Page/provider,
normal API action/client, registered notifications, normal client navigation,
and actual picker. External transports alone are isolated; backend event
production and browser geometry are not claimed. There is no Playwright
file/project or whole-app build in this work package.

Mobile parity classifies this as pure data/state correction. Phone
`AppNavSheet` consumes the shared picker and catalogue. Existing confirmation
surface, composition, touch targets, scrolling, navigation and breakpoints are
preserved. The skill permits focused component/provider evidence for this
case; no new mobile test or ASCII preview is needed. Any rendered or route
change invalidates that exception and must checkpoint ROOT.

## Work orders

- [x] [Task 01: Preserve accepted deletion catalogue](task-01-preserve-delete-catalogue.md) — local implementation and checks complete, wave 1, no dependency; hosted review and merge pending.

## Discovery, phase and resources

Task `06f5b683-b60e-4891-8715-593b6fe66ad7`, primary session
`ca3b7e1b-e719-44ee-a1e2-6064870afd66`, CHILD107; parent ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Baseline
`0651c4faa50e19c356100bbf5fe6fe4204f50b10`, branch
`feature/preserve-live-worksp-jzw`, initially clean worktree.

Accepted ROOT metadata at
`/tmp/kandev-root-workspace-delete-discovery-20261010/qualified-proof.json`
reports one mixed-state causal RED, two controls PASS, exit 1, actual original
join and empty owned group, temporary source removed and ROOT clean. No proof
replay is needed. Protected `candidate.test.tsx` remains ROOT-owned and must
never be read, copied, imported, replayed, changed or deleted.

Confirmed intent and source resolve the local scope; no material product
question remains. The existing broad Office redirect criterion is not changed
to fit this Settings path. No new ADR is necessary for the established
current-provider idiom.

The design turn ended with exactly four unstaged/uncommitted artifacts and
DESIGN_READY. ROOT read the full actual package and later explicitly released
implementation in the same primary with the exclusive GLOBALheavy107 grant.
The sole work order's local implementation and scoped checks are complete.
Normal hooked publication, hosted review and the separate merge grant remain
the subsequent delivery gates.

After release, install from `apps/` once with frozen lockfile before package
checks; run the one order's RED, minimal correction, GREEN and exact checks
serially under the grant. Retain and join original handles across interrupts,
never duplicate commands or discard process state. Preserve managed
dependencies/worktrees/shared caches, foreign refs/processes/protected data
and the paused oversized task. Resource failures or required scope expansion
checkpoint ROOT before retries or alternatives.

Normal hooks, commit, push and ready PR follow passed local checks under the
existing delivery authorization. Return GLOBALheavy only after actual local
joins and current known own-process absence; proceed to hosted observation in
that same turn. There is no extra publication ceremony or mandatory idle phase.
Use one retained 90-minute `pr-await` with GNU 91-minute TERM/kill-after-10s
bound and about 60-second progress; follow new heads without restarting solely
for pushes. Read automated review while CI runs. Reuse substantive review plus
bounded later diffs; request focused rereview only for material correctness,
security or architecture. Optional polish does not justify another push/gate.

Merge requires a separate ROOT serial grant after the six known required
contexts and actual Backend/Frontend/E2E workflow parents succeed at the current
head, fresh complete/error-free evidence, zero actionable visible/hidden/human
findings and clean head with substantive review plus later diff. ROOT owns one
static compatibility check; merge normally with expected head and squash, no
admin bypass. Verify actual merge and owned cleanup, then ROOT independently
fast-forwards/archives. Job-name retry budgets persist across heads; no blind
retries/cancellation, empty CI refresh, drift-only rebase or passing broad replay.

## Verification results

Lightweight design checks passed: the catalogue validated 368 decisions and
1,501 specifications; all specification files passed lint and the linter's 36
tests passed. Real PR-docs `validateCoverage` classified the actual four
documentation paths as exempt and the separately labelled prospective
production-reference preflight as covered, accepting this work order/design
with zero errors. Whitespace passed. The four files were unstaged/uncommitted at handoff;
requirement 001's existing section remains byte-for-byte unchanged.

The shell did not expose `node` on PATH; the dependency-free docs preflight
used the already-installed Node at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`. No installation or
frontend runner was used. Future package commands use the repository's existing
mise tool environment, only after the implementation and resource grants.

Independent pre-fix RED reached two post-acknowledgement failures (mixed live
membership and survivor descriptors) with nine passing controls, including
ordinary and rejected deletion. The initial empty-executor fixture failed
Radix setup and is not RED evidence. The completed fixture uses actual feature
defaults and typed producer payloads; no product mocks or protected proof reuse.

Final local results: all 11 regression cases pass; the four affected existing
request/event/slice/picker suites pass all 65 tests; scoped ESLint, Prettier,
web typecheck and i18n checks pass. Fixture-only typing, callback-length and
format repairs were followed by the affected new suite/lint/typecheck checks.
The production diff remains confined to `workspace-edit-client.tsx`.
No build, broad suite, browser, database or unrelated writer was changed/run.

The [work order](task-01-preserve-delete-catalogue.md#results) records commands
and outcomes. Original wrapper metadata/raw logs are retained under
`/tmp/kandev-child107-runs/`; the own task plan holds native session/join receipts.
The first install's native handle was omitted by tool output projection; its
original wrapper recorded actual process join, exit 0 and empty owned group,
and the install was not duplicated. All later full tool responses were retained.
Hosted CI/review, separate ROOT merge grant and owned cleanup remain pending.

## Documentation assessment

Internal docs updated: the owning deletion requirement/design and this two-file
package. Public owner-only deletion/manage guidance in
`docs/public/team-access.md`, workspace procedures in
`docs/public/tasks-and-workflows.md`, root README and screenshots remain accurate.
No operation, label, settings value, public API or image changes.

## Risks

- A current-store read before await reproduces the defect; publication must
  read after acceptance and remain synchronous through the setter.
- The slice preserves a non-null active target ID until existing event/bootstrap
  policy resolves it; do not expand this fix to selection or navigation repair.
- Mocking product boundaries or failing in setup would conceal the causal issue.
- Same-ID delete/recreate chronology and global list freshness remain excluded.
