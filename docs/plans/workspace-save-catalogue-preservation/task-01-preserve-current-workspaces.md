---
id: "01-preserve-current-workspaces"
title: "Preserve current workspaces at save acknowledgement"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-002
acceptance_criteria:
  - AC-WORKSPACES-SETTINGS-UPDATES-002.1
  - AC-WORKSPACES-SETTINGS-UPDATES-002.2
  - AC-WORKSPACES-SETTINGS-UPDATES-002.3
  - AC-WORKSPACES-SETTINGS-UPDATES-002.4
  - AC-WORKSPACES-SETTINGS-UPDATES-002.5
  - AC-WORKSPACES-SETTINGS-UPDATES-002.6
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
---

# Task 01: Preserve current workspaces at save acknowledgement

## Summary

Independently prove and repair catalogue publication after acknowledged settings
Save. Keep accepted target settings while preserving current other-workspace
choices and target metadata. Execute sequentially only after the later explicit
ROOT implementation interrupt and exclusive heavy lease in the original primary.

## In scope

- Read the owning pair, plan, scoped AGENTS, `/tdd`, and the actual production
  source; repeat the bounded read-only caller inventory at release.
- Author permanent real Page/provider/coordinator/API/registered-WS/sidebar
  regressions independently; never read, copy, import, replay, edit, chmod or
  remove protected ROOT proof/fixture .tsx sources.
- Change `workspace-edit-save.ts` and only immediate `useWorkspaceEditForm`
  wiring in `workspace-edit-client.tsx` to supply/read the owning current store
  at successful acknowledgement. Keep the exact accepted-target projection.
- Preserve existing API presence, defaults, scopes, dirty/baseline/contributors,
  rejected saves, newer drafts, and real navigation. Record exact evidence and
  lifecycle/delivery outcomes in this work order and plan.

## Out of scope

No delete/create/placement repair, corrected-caller migration, target revision
arbitration, permission redesign, backend/API/schema/global cache/revision change,
store or picker implementation change, product mocks, proof reuse, optional polish,
new work order/delegate/session/model, or automatic implementation from design.

## Acceptance

1. Independent real integration overlaps establish causal RED on addition,
   update and removal with both store and real choices reached, accepted target
   successful, and strict passing success/rejection controls. Metadata preservation
   claims receive independent causal evidence. No fixture error counts as proof.
2. The same bounded matrix passes after the two-file current-store repair and
   covers all 002.1-.6 without widening accepted-target fields, payload presence,
   permissions, contributor/draft/error behavior, or navigation.
3. Exact scoped checks pass within released bounds, receipts are actually joined,
   owned groups are freshly absent, and public/mobile/lifecycle assessments remain
   accurate. Hosted/publication/merge require their separate ROOT release gates.

## Independent integration fixture and sequence

Use `workspace-edit-save.integration.test.tsx`, with a colocated
`workspace-edit-save.test-helpers.tsx` only if fixture size requires it. Import
real `WorkspaceEditPage` (`[id]/page.tsx`) and let it render the real edit client.
Use real StateProvider/createAppStore, routing/history and navigation guard,
ToastProvider, TooltipProvider, SettingsSaveProvider and AppSidebarWorkspacePicker.
No `vi.mock` of product modules, stores, handlers, actions, hooks, coordinator,
router, UI primitives, or picker content. A test observer may capture the real
store API/coordinator and invoke actual `saveAll`, not emulate contributors.
Seed ordinary active executor and agent-profile options, server-issued manage
scope, feature state and real workspace metadata. Do not recreate the earlier
empty-active-executor fixture error or manufacture a disabled form.

Intercept external fetch only with strict method/path/payload admission. Return
legitimate metadata/org-unit/profile/settings reads required by real mount;
unexpected transport is an error. Hold PATCH with a finite, explicitly settled
promise ticket, not a timer. Establish actual routing state and assert it through
real router/history/links; no router spies. Deliver external WS stimuli through
actual `registerWorkspacesHandlers` on the observer's owning provider store.
Do not hand-construct the post-event catalogue or emulate handler behavior.

For each first-three matrix case: edit the actual form; start real coordinator
Save; await PATCH admission; deliver the relevant registered event while it is
pending; open the real picker and assert current row/name or absence plus current
store values before release. Resolve the successful PATCH and actually join Save.
Assert accepted target, current catalogue/order/other fields and actual picker
choices after publication. Store and menu assertions must both be evaluated in
the causal pre-fix failing evidence (soft assertion grouping may ensure both are
reached; final assertions remain strict). No unhandled errors or unexpected
transport may contaminate the failure. Addition additionally selects the retained
choice after clean Save and verifies real selection/navigation. Update and removal
use independent distinguishable other-workspace data, not the addition test alone.

For metadata: during PATCH deliver a target `workspace.updated` with changed
nonprojected description, unit, configuration default and timestamp. Keep initial
owner/scopes/created values distinguishable from a conflicting accepted response;
where current scopes/owner changes are asserted, use the real owning hydration
boundary without pretending WS carries fields it does not support. Change active
selection through actual `setActiveWorkspace`, capture its revision, and confirm
Save preserves both. Accepted response projected defaults and idle values must
still take effect; description/owner/scopes/unit/config default/timestamps must
remain the current row's values. Do not claim new target-revision arbitration or
target-deletion navigation coverage.

Implement all named cases in [the plan matrix](plan.md#tests-and-traceability).
The success control verifies baseline through clean contributor and a subsequent
edit/payload comparison, ordinary route, and absent/default projection behavior.
Rejection combines live addition and newer Name, verifies failure feedback and
contributor, no accepted projection/baseline, dirty-state and actual route blocker.
A separate accepted/newer-Name control verifies submitted value in the catalogue,
newer draft in the field, dirty contributor and canLeave=false. Parameterized
payload capture uses real control events for changed keys, trimmed Name, clearing
both default selectors, explicit idle false, numeric timeout, mixed edit and
omitted unchanged keys. Pristine/invalid/manage-scope controls cannot send PATCH.
These remain client presence controls; no backend execution claim is added.

Keep all tickets/Save promises owned, settled and joined in success and failure
cleanup; unmount real providers, unregister test observations, restore external
fetch/history/guard/global state. No sleeps, synthetic extra passes, timeout
weakening or leaked background work. A causal assertion failure permits fixture
correction and affected rerun; resource/transport/unknown failures checkpoint ROOT.

## Files likely touched

Production ownership is exactly:

- `apps/web/app/settings/workspace/workspace-edit-save.ts`
- `apps/web/app/settings/workspace/workspace-edit-client.tsx` (immediate Save wiring)

Permanent regression ownership, independently authored after release:

- `apps/web/app/settings/workspace/workspace-edit-save.integration.test.tsx`
- `apps/web/app/settings/workspace/workspace-edit-save.test-helpers.tsx` (conditional)

Delivery documentation ownership is the owning requirement/design pair, this
work order, and `plan.md`. Other production and tests remain read-only inputs.
Existing unit tests with mocked primitives are retained compatibility evidence,
not substitutes for the new real integration matrix.

## Dependencies

No task dependency. `apps/node_modules` and `apps/web/node_modules` absent at
design. Existing mise Node24.21.0 is verified; use its bin PATH under Bash
`login:false`. Only after later release, if still absent, run one pinned frozen
install from `apps/` (no runtime download/substitution):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 10m pnpm install --frozen-lockfile)
```

The command requires the existing pinned `pnpm` launcher: first confirm
`pnpm --version` is 9.15.9. An absent/mismatched
launcher checkpoints ROOT before any alternative/download. One successful install
satisfies the dependency gate; do not repeat it for passing checks.

## Verification after later release

Run each command serially in its own native handle under Bash `login:false`,
from repository root with the existing Node bin prepended to PATH. Record argv,
cwd, UTC, PID/PGID, bounds, original initial+terminal receipts and exit; actually
join each and freshly observe owned groups gone. These commands were excluded from the original DESIGN turn and explicitly
released later by ROOT. The one conditional install completed before pnpm checks;
its actual pinned version and initial probe anomaly are recorded in the plan.

Causal RED: new overlap/metadata regressions and success/rejection controls on
pre-fix production; record the exact assertion failures and controls:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m pnpm exec vitest run app/settings/workspace/workspace-edit-save.integration.test.tsx --maxWorkers=1)
```

GREEN after production edits uses the same exact command. Run compatibility
checks once after GREEN, not a broad suite:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m pnpm exec vitest run app/actions/workspaces.test.ts components/settings/settings-save-provider.test.tsx lib/ws/handlers/workspaces.test.ts components/app-sidebar/app-sidebar-workspace-picker.test.tsx --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 5m pnpm exec eslint app/settings/workspace/workspace-edit-save.ts app/settings/workspace/workspace-edit-client.tsx app/settings/workspace/workspace-edit-save.integration.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 2m pnpm exec prettier --check app/settings/workspace/workspace-edit-save.ts app/settings/workspace/workspace-edit-client.tsx app/settings/workspace/workspace-edit-save.integration.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If the optional helper is authored, include that exact path in scoped eslint and
prettier arguments; it is imported by the integration test. Do not add an all-file
lint, build, full Vitest, E2E, broad verification or passing replay. Rerun only
checks affected by a routine causal fixture/lint/format correction. Typecheck's
existing pre-step generation is part of the released check, not design work.
Normal active commit hooks remain intact; ROOT's reviewed implementation release
authorizes ready publication after these task-defined checks.

## Mobile and public documentation

[Design exception and public assessment](../../specs/workspaces/system-design/workspace-settings-updates.md#independent-client-verification-mobile-and-documentation)
apply only to this state/data publication repair. No rendered composition/copy/
touch/scrolling/navigation/breakpoint edits are admitted, so no ASCII preview,
product build or new mobile Playwright test is planned. Real shared picker
integration provides the causal user outcome. Any surface change checkpoints ROOT.

## Parallelism and standing gates

`sequential`. No native delegation, new persistent task, tab/session or model
change. [All plan gates](plan.md#phase-and-resource-gates) apply, including exclusive
heavy lease, actual joins/fresh group absence/RETURN, caller-bound canonical ready
PR/all five automation flags false, frozen head, separate hosted release/one original
observer, full current all-file substantive App347564 review, six contexts plus actual
parent SUCCESS, and separate serial compatibility/normal squash merge grant.
Preserve managed/foreign/paused resources. Critical parent question ends turn;
optional queued callback/full queue is never a gate or child-to-parent interrupt.

## Inputs

- [Requirement 002](../../specs/workspaces/requirements/workspace-settings-updates.md).
- [Current catalogue publication design](../../specs/workspaces/system-design/workspace-settings-updates.md#current-catalogue-publication).
- [Traceability matrix and evidence limits](plan.md#tests-and-traceability).
- Read-only actual Page/provider/coordinator/API/WS/picker source, Office appearance
  current-store idiom and existing repository/Save tests. Never ROOT proof source.

## Results

Local implementation and task-defined checks are complete under ROOT's later
reviewed release. [Actual results and retained failures](plan.md#released-implementation-results)
record independently authored causal RED, all20 matrix cases through the19-pass
run and affected navigation correction,79 compatibility tests, affected5-case
helper validation, clean scoped lint/format and final typecheck. The real
provider store is read after acknowledgement; accepted-target fields remain
exactly bounded to the prior projection. Public/mobile assessment holds for
the actual two-file production diff.

Status remains `in_progress` for ready publication, explicit joined LOCAL-HEAVY
RETURN, ROOT qualification, and separately released hosted/merge gates. No hosted
or merge completion is claimed. Exact receipts are retained in the task plan
and `/tmp/kandev-child94-runs/`; no ROOT .tsx proof source was accessed.
