---
created: 2026-10-06
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002
system_design:
  - ../../specs/integrations/system-design/workflow-sync-settings-lifetime.md
legacy_specs: []
---

# Retire workflow sync navigation with its settings view

## Overview

One sequential work order fences local Workflow Sync publication, pending
settlement and immediate dialog completion at the committed settings lifetime.
Requirements extend the existing
[integration owner](../../specs/integrations/requirements/gitlab-workflow-sync.md#req-integrations-gitlab-workflow-sync-002-settings-lifetime);
the [design](../../specs/integrations/system-design/workflow-sync-settings-lifetime.md)
owns the boundary. The older GitLab-provider package remains done with unchanged
scope and results; no task in that historical package is reopened. The tasks
workspace-authorization package is an independent contract and remains unchanged.

The four-artifact design handoff was reviewed by ROOT, which released Task 01
in this same primary session, profile and executor and granted the exclusive
global local-heavy lease. Initial implementation and the reviewed removal-success target-change
correction are complete; delivery checks continue and merge is not released.
No delegation, task/session/tab or model change. The three-fix program does not
authorize parallel child work.

## Verified input and evidence limits

Live audit at `5d3bc32cd240d36c1c5f780f64890172f4836f9d` matches all three
accepted blobs from proof base `c373ba436c3451f046acd921d5de9a081977a2a7`:

| File | Blob |
| --- | --- |
| `apps/web/hooks/domains/settings/use-workflow-sync.ts` | `9f18a937da3781dafa36562c1272b0c74d9a17b7` |
| `apps/web/components/settings/workflow-sync-section.tsx` | `f54fcde83c822f792e9569de6d89907f36e0de4f` |
| `apps/web/lib/api/domains/workflow-sync-api.ts` | `e85543b29137d2008a984f25ee9e304da03bcd10` |

Accepted private references are read-only:

- `/tmp/kandev-root-workflow-sync-retired-navigation-proof-classification.json`,
  `/tmp/kandev-root-workflow-sync-retired-navigation-proof-receipt.json`, and
  `/tmp/kandev-root-workflow-sync-retired-navigation-proof.log`: original
  85550 / 9760d8 ACTUALLYJOINED dc520c exit 1, 5.744s. Two causal failures:
  retired A removal and forced sync refresh global navigation after actual
  A unmount/B mount. Two passing current-operation controls. B config remains
  B and successful retired removal still truthfully resolves true.
- `/tmp/kandev-root-workflow-sync-retired-navigation-next-candidate.test.tsx`,
  mode 0400, SHA256
  `be573e9ea2fce820e6d3f4ae0dfa7e12521f56e17617d4cdb51d97f2efac84c0`.
- `/tmp/kandev-root-workflow-sync-owner-proof-classification.json`,
  `/tmp/kandev-root-workflow-sync-owner-proof-receipt.json`, and
  `/tmp/kandev-root-workflow-sync-owner-proof.log`: original
  15282 / 1d5dc3 / a46ee6 exit 1; two same-hook failures/two current controls.
  This establishes limited hook API fragility, not actual B-config overwrite
  through the ancestor. The supplementary classification supersedes that
  possible overclaim; the historical evidence remains untouched.
- `/tmp/kandev-root-workflow-sync-owner-next-candidate.test.tsx`, mode 0400,
  SHA256 `c1f03a609467937624b60e6ee96e5e689197ee984826d7ee731962a1e8067e11`.

Neither proof is replayed, copied, imported, mutated or removed. Original
receipts report all processes/groups gone, scratch removed and ROOT clean.
Use independent fixtures later. Moving main alone does not justify rebasing
or replaying accepted evidence. Full route/dialog behavior was not proved by
the original tests. Source establishes the ancestor unmount and save caller
gap; the latter requires new causal integration coverage during implementation.

## Scope and approach

Own `useWorkflowSync` admission and settlement for initial/background GETs,
save, removal and force sync; committed form-input callbacks; local form/config,
feedback and pending controls; and the immediate save/remove dialog completion.
Reuse hook-local committed identity/activation and separate control tickets.
Keep Boolean save/delete and void force outcomes truthful after dispatch.
Protect the dialog's completion epoch without treating its own save response
as replacement. `WorkflowSyncSection` may pass local controller identity if
needed; it retains the keyed workspace dialog and the existing composition.

Exclude backend/DB/schema/writer changes, authorization, transport abortion,
global caches/frameworks/events/version ordering, generic stale-request cleanup,
new draft-edit policy, timer/cadence changes, runtime flags, copy and layout.
Existing successful navigation remains the same. No root/scoped guide change
is needed: package structure and engineering conventions remain accurate.

| Provider / transport | Identity | Preserved behavior | Coverage |
| --- | --- | --- | --- |
| GitHub / fetchJson | workspace + repo_owner/repo_name | Existing trimmed payload, current outcomes and reload rules | Transport payload/current controls and retirement matrix |
| GitLab / fetchJson | workspace + project_path; host stays backend-owned | Existing payload and provider-specific form/reset | Same local lifetime matrix; first save/provider switch dialog controls |
| Other provider | Unsupported by existing types/API | No newly supported provider or fallback | No new coverage claim |

## Tests and acceptance mapping

All rows refer to `AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.<N>`.
The mapping below originated in design. Actual executed coverage and receipt
counts are recorded in Verification results.

| AC | Prospective file and named scenario |
| --- | --- |
| .1, .5, .6 | `apps/web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx`: `retired removal remains true without reloading B`; `retired force completion cannot reload B`; current changed/unchanged/error controls |
| .1, .2 | Same file: `retained retired callbacks dispatch nothing`; initial/background GET success/reject before passive cleanup; old input callbacks cannot edit current form |
| .3 | Same file: `A-B-A never reactivates old A`; `independent views retain admission`; speculative render and StrictMode cleanup/reactivation |
| .4 | Same file: `older save finalizer leaves newest save pending`; equivalent sync/loading tickets, replacement finalizer success/reject |
| .5, .8 | Same file plus existing hook/API tests: admitted writes keep original workspace/payload and caller result; same-workspace current save/reset and silent status-only GET |
| .7, .6, .8 | `apps/web/components/settings/workflow-sync-section.lifetime.test.tsx`: real section/hook/dialog/providers/API, closed/reopened/unmounted dialog, retired save no dismissal, current first save/provider switch/removal dismiss, desktop/phone failures and removal retry |

Preserve existing suites `use-workflow-sync.test.ts`,
`workflow-sync-dialog.test.tsx`, `workflow-sync-status-banner.test.tsx`,
`workflow-sync-api.test.ts`, and `settings-routes.workspace-data.test.tsx`.
No hook/provider/store/API-client mocks in new causal suites. Only fetchJson
and the actual refresh boundary may be mocked; resolve deferred transport at
asserted lifecycle boundaries and drain every owned pending/timer at teardown.

## Rendered flow and mobile exception

The real section/dialog integration suite supplies user-facing end-to-end
evidence within the component-to-transport boundary; the hook suite separately
tests actual unmount/remount. It does not claim full browser-route coverage.
No new Playwright test, build or local E2E run is required for this bounded
state/data fix. Mobile-parity explicitly permits targeted unit/component
coverage for state-only work in the unchanged shared surface. Exercise the
existing desktop and phone dialog branches. There is no proposed layout to
preview, so ASCII UI previews are omitted.

Public audit: `docs/public/workflow-sync.md` is a workflow configuration guide
with API reference; root README and screenshot catalog expose no changed
contract. Save, Sync now, removal, API shapes and cadence remain accurate.
No new guide prose or UI translations are proposed. Reaudit the actual diff.

## Work orders

- [x] [Task 01: Retire settings completions](task-01-retire-settings-completions.md)
  (`done`, wave 1, sequential, no dependencies).

## Verification results

Design checks passed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: 357 decisions and 1,411 specs validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Catalog query discovers the existing owner and the new bounded design.
- Markdown links/anchors, four-artifact whitespace, REQ/AC definitions and
  work-order/plan/design inclusion passed; exactly one pending order exists.
- Repository `validateCoverage`: actual design-only paths are exempt;
  explicitly prospective production paths are covered by this work order,
  with `errors: []`. This is reference validation, not executed code coverage.
- `git diff --check` passed and the index is empty.

Exact later serial commands and process receipts are in Task 01.
Implementation checks passed on 2026-10-06:

- Final causal hook suite: 29/29; real section/dialog: 15/15. Five adjacent
  hook/dialog/status/API/ancestor-route suites: 38/38. These are 82 distinct
  checks across the original selection and affected repair runs, not one run.
- Independent causal REDs include actual retired reload, retained admission,
  pending ownership and dialog dismissal. A faithful root StrictMode RED against
  the original committed hook proves first-setup callback retirement; suspended
  speculative-callback RED proves commit admission. Fixture/setup corrections
  are classified separately in Task 01.
- Changed lint/format, project typecheck with normal prehook, i18n check/ratchet,
  catalog/all-spec lint/36 lint tests, local links/anchors and whitespace pass.
  Actual diff documentation coverage is `covered`, with `errors: []`.
- Public guide, README, screenshot catalog and scoped/root guidance remain
  accurate. Mobile data-only exception is exercised at 390 and 1024 widths.
- All originals are ACTUALLYJOINED and PID/groups gone. Detailed private
  command receipts are `/tmp/kandev-child59-<label>.json` and `.log`; Task 01
  lists the concrete labels. Protected original proofs remain untouched.

Local implementation is complete. Ready publication, hosted evidence and the
separate ROOT serial merge release remain delivery barriers.

## Risks and delivery barriers

The main risks are passive-cleanup timing, StrictMode reactivation, accidentally
masking successful backend mutations, own-save target changes preventing dialog
dismissal, and finalizers clearing newer controls. Causal tests must distinguish
each from framework setup failures. No same-workspace draft-edit contract or
global ordering may be inferred from the accepted sentinel.

After later authorized implementation and task-defined checks, standing
authorization covers commit/push and a ready PR with normal active hooks and
no bypass. Return the local-heavy lease at an explicit `heavyRETURN` checkpoint
only with a clean aligned exact head and every original joined/gone. Start ONE
original 90-minute all-terminal hosted observer under GNU timeout 91m/kill10;
retain its native handle across findings and corrective head updates, with no
duplicate collector or self-successor. Deadlines/lost results are NO VERDICT.
Freeze the SHA except actual corrective findings; no moving-main rebase,
synthetic tests, optional polish or uncontrolled hosted reruns.

Require six known required contexts and actual Backend/Frontend/E2E parent
success, fresh complete `errors[]`, clear visible/hidden/actionable/human
findings, and authenticated configured CodeRabbit App347564 substantive full
current-head coverage of every changed file. Accept sufficient automatic
review; inspect skips/gaps before ONE necessary full request, no ACK or optional
duplicate review. End `MERGE READY`; wait for a separate ROOT SERIAL MERGE
INTERRUPT. Then use expected-head normal squash, independently verify actual
merge commit/tree, owned contracts and remote inclusion, and join only-owned
cleanup. Task completion requires verified merge and all owned closure. ROOT
owns archiving, proof release and refill; preserve managed worktrees,
dependencies, protected proofs and foreign resources.


The reviewed corrective release accepted duplicate Greptile/Claude findings at
`2fb6aad5`: a genuine pending-removal target change bypassed success dismissal
admission. Task01 records causal RED for four actual consumer cases and GREEN
for 28 affected dialog tests. A private generation snapshot before removal's own
reset preserves the successful current dismissal and truthful admitted outcome.
The initial hosted observer joined at deadline without a verdict; ROOT's
acceptance receipt permits exactly one new-head observer after corrective
commit/push and clean aligned heavyRETURN. Merge remains a separate ROOT lease.
