---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Preserve profiles during dynamic saves

## Overview

A standalone dynamic-profile save currently rebuilds shared profile collections
from the agent list captured before its HTTP request. A profile delivered by
WebSocket during that request can disappear when it completes, and a valid
flattened option whose Settings owner is absent can become Unavailable in the
actual picker. One sequential work order will independently write the
regression, confirm RED, then publish the owned response against current state
using existing reconciliation and confirm GREEN.

ROOT reviewed the four design artifacts and sent a later explicit same-primary
implementation release. The bounded implementation and scoped product checks
and documentation validation are complete; normal delivery is in progress.
GLOBAL LOCAL-HEAVY is exclusively CHILD56 until explicit return after ready PR
publication. MERGE remains NONE; a separate ROOT interrupt is required.

## Scope

### In scope

- `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.9` in the existing Agents requirement.
- Current-state publication by the real standalone dynamic editor, including
  every unrelated profile and current flattened option.
- Existing target revision, absence, draft and acknowledgement behavior.
- Independently authored real-state/coordinator/registered-WS/picker regression
  and positive controls; exact verification in the work order.

### Out of scope

- Other-profile editor save rewrites and the unused `useProfileEnabledToggle`.
- API, backend, persistence, SDK, permissions or transport changes.
- New synchronization/cache mechanisms, retries, timers, ordering rules or
  owner-lifetime guarantees.
- Runtime flag registry/defaults/profiles/settings or rollout changes.
- Layout, copy, touch, navigation, scrolling, breakpoints, browser setup, E2E
  and build work without new causal evidence and an explicit resource release.
- Delegation, new tasks/sessions/tabs, model changes or operator approval prompts.

## Evidence and assumption check

ROOT proved the production consumer on main
`95c040e84951773dc8ed6f684fff0425d7b63f96`. Its accepted final run
`11785/c8de14/9272e0` actually joined at `2026-10-06 17:50:41.811 UTC`, exit 1:
two causal failures and two positive controls passed, four tests in 9.60s.
The proof used real StateProvider/createAppStore, ToastProvider,
SettingsSaveProvider/useSettingsSaveCoordinator.saveAll, dynamic editor hook,
registered WS handling, API action/normalizer and AgentProfilePicker, with only
a partial fetchJson mock. The supported oracle is identity, option `label`,
enabled state and revision; optional normalized fields are not a loss oracle.
Accept this evidence without replaying or copying its fixture.

Protected read-only ROOT archives and hashes:

| Archive under `/tmp/` | SHA256 |
| --- | --- |
| `kandev-dynamic-profile-save-concurrency-supported-control-candidate.test.tsx` | `eb0498b45a90050687b6269067cd679bd203bcad93f05b7c28d8387c4a2371ce` |
| `kandev-dynamic-profile-save-concurrency-candidate.test.tsx` | `3816e3f190b2a3a66212804f37b55b1d20a8ed9138724ecd9501ae1548e0ce4a` |
| `kandev-dynamic-profile-save-concurrency-corrected-control-candidate.test.tsx` | `8cefaa41ac75258339b979402431b30587d12fb8de25de4076b419a6cf79aa20` |

Do not read for fixture reuse, replay, copy, mutate or delete the archives,
classification `/tmp/kandev-root-dynamic-save-proof-classification.json`, or
ROOT's original logs/receipts. ROOT reports every original PID/PGID gone and the
disposable test removed. This design turn runs no proof process.

Confirmed intent and exclusions come from ROOT. Source inspection verifies the
captured-list defect, existing helper semantics and production caller. Agents
owns profile values and projections; Platform's
[editor reconciliation](../../specs/platform/system-design/agent-settings-parity.md#editor-reconciliation)
owns the shared interface contract. The narrow repair restores these existing
boundaries; no new architectural decision or duplicate UI authority is needed.
No material question remains unresolved.

## Technical approach

Read `useAppStoreApi().getState()` after the real update action resolves. Map
the latest `settingsAgents.items` to replace only the existing matching profile
under the existing editor owner, subject to `isProfileRevisionNewer`. Do not
insert absent profiles or owners. Reuse `reconcileAgentProfileOptions` directly
or via `useSyncAgentsToStore` so unrepresented and newer options survive.
Keep the publication synchronous and preserve existing draft acceptance,
payload, coordinator, errors, toasts, saved revision and embedded callbacks.

Production ownership is
`apps/web/components/settings/dynamic-agent-profile-editor-state.ts`.
`agent-profile-page-state.ts` is optional ownership only if the existing helper
genuinely needs a bounded adjustment; prefer reuse without modification. The
draft, store, WS handler, picker and action/normalizer are evidence inputs,
not production edit targets.

The owning [design section](../../specs/agents/system-design/dynamic-agent-routing-01.md#standalone-settings-save-publication)
defines revision ties and missing-row behavior. Newer targeted revisions must
use established rules; this package promises no global deletion, movement or
server compare-and-swap policy.

## Tests

Author
`apps/web/components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx`
independently. Use the actual store/providers/coordinator, hook, registered
   `registerAgentsHandlers` and picker; preserve the real action/normalizer and mock
only fetchJson as needed. Hold the PATCH promise open, deliver WS through the
registered handler, assert accepted state before completing the request, then
assert the resulting store and rendered picker. No sleeps or helper-only
predicate oracle. The work order names each anchored test and its control.

| Contract | Evidence |
| --- | --- |
| AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.9 | Known-other-owner create/update/delete during deferred save; mixed represented/unrepresented options; retained actual picker label; failed response without destructive publication |
| Existing target reconciliation | Newer target revision and absent target/owner; current ACK and own WS ACK controls |
| Existing AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.2/.6 and settings behavior | Ordinary successful request/payload, valid-policy submission, invalid-policy coordinator veto, parent-owned embedded draft |

Existing action/payload, profile-reconciliation, options, picker and coordinator
tests remain controls. Use their exact work-order command, not a full suite.

## Mobile and rendered verification

This changes shared state/data publication inside an existing editor. It changes
no rendered composition, interaction, geometry, copy or viewport branch. The
nearest shipped surface is the current direct dynamic-profile route and
`dynamic-agent-profile-editor.tsx`; both viewports consume the same corrected
state. Under mobile-parity's pure state/data exception, the targeted real-picker
component regression supplies rendered outcome evidence. No ASCII preview or
new Playwright run is needed. Reopen this assessment only if implementation
actually changes a presentation boundary, and end to ROOT before expanding.

## Public documentation

Audited `docs/public/agents-and-profiles.md` (dynamic profiles and existing save
guidance), root README and screenshot catalog. Instructions, labels, entry
points, feature defaults, API and workflow behavior stay accurate. Internal
docs are updated; no public-doc or screenshot change is needed for this repair.

## Work orders

- [x] [Task 01: Reconcile standalone dynamic saves against current profiles](task-01-current-profile-publication.md)

One wave, one work order, sequential, no dependencies. No delegation.

## Verification results

Design checkpoint on 2026-10-06:

- `python3 scripts/list-docs.py validate`: exit 0; 357 decisions and 1408 specifications.
- `python3 scripts/lint-spec-files.test.py`: exit 0; all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specification files passed.
- Repository `pr-docs.cjs` validator: actual four-file documentation diff is
  exempt; separate declared-source ownership preflight is covered with
  `errors=[]` and the actual requirement/design/work-order references accepted.
  This is artifact validation, not a claim of production implementation.
- Four artifacts and 24 local Markdown links checked: targets/anchors exist,
  files stay under 32KiB, no trailing whitespace or conflict markers.
- `git diff --check`: exit 0. Exactly the owning pair and this two-file package
  are changed, unstaged and uncommitted.

The independently authored regression failed causally before the hook correction.
All 13 new cases and 54 existing controls pass; changed-file ESLint, normal
project typecheck, i18n check and ratchet pass. Own fixture type/lint repairs
are recorded in [Task 01 results](task-01-current-profile-publication.md#results).
Actual six-file reference coverage is covered with errors=[]; 25 local links,
32KiB ceilings and whitespace pass. The initial design-check history above is
retained. Commit/publication, CI and
review readiness are pending; merge is separately gated.

## Risks

- `isProfileRevisionNewer` handles equal timestamps through editable equality;
  option merges use rebuilt ties. Tests must reflect both existing rules.
- Complete optional-field equality can create false failure after normalization;
  use supported preservation fields and normalizer-backed ACK fixtures.
- Known-owner and absent-owner rows require a mixed fixture to detect option loss.
- The dynamic contributor currently catches errors; characterize actual error
  and coordinator behavior without changing that separate contract.
- Resource/setup/timeout/transport/unknown or out-of-scope failures end to ROOT;
  no self-directed retries or scope expansion.
