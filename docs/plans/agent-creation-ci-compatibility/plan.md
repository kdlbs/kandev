---
created: 2026-10-10
status: in_progress
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
legacy_specs: []
---

# Implementation Plan: Agent creation CI compatibility

## Overview and checkpoint

Restore accepted new-agent publication after an independent profile event, and
reconcile ten concrete test compatibility failures with the existing profile-order
implementation. Agents owns configured identities/membership/choice projection.
Reuse unchanged [creation catalogue requirement](../../specs/agents/requirements/creation-catalogue.md),
all six ACs; minimally amend its design. ONE sequential order, distinct from
[scroll remediation](../preserve-prompt-jump-alignment/plan.md) and the committed
[Global fix](../preserve-saved-global-secrets/plan.md).

DESIGN ONLY. ROOT full actual-file review and LATER explicit implementation
INTERRUPT are required before Agents production/permanent tests/source integration
or runtime. SAME task450a75ba-e760-464e-b185-2b3eb1f407b4/session
da87b7fb-c6ac-4d31-8a41-df529c2e5237/primary; no delegates/new sessions. Scroll
alone is released under heavy111. No publication until required corrections pass.

## Exact evidence and integration boundary

PR4398 head a589325537d2b70ff66f02f289c1d1ad62f88127 was tested by Frontend
run38037433681/attempt1/job114170812599 at merge
0443cfa0d60fdb1037a698ce038a97aa5e661eff. Its full final log has12 failed cases
in5files, with25215 passing cases. Original hosted native42336 joined1/4a50c0;
wrapper/group absent, no old-head retry. Exact authenticated source comparisons
against authoritative refs/heads/main02ff0578357040b0546ea17cd9a00dff9ca9ee3b
prove all five failing tests and eight implicated production blobs unchanged
between tested merge and observed main. Receipts/full log/classification are under
`/tmp/kandev-global-secrets-design-20261010/`. No cause is attributed to Global hook.

Precise product cause: `saveNewAgent` invokes `upsertAgent` without creation
context on success and accepted-create/failed-MCP. Tested/main creation sync sends
that ordinary path to `syncSavedAgentToStore`; its missing-owner/version gate
returns after the independent event increments the admission version. The accepted
new owner disappears from publication. The earlier `creation && !current` causal
attribution was imprecise and is corrected in the durable classification; that
existing-owner guard is not the new-agent call path.

Candidate differs from tested/main: it lacks `agent-save-store-sync.ts`, version
capture/delegation, and profile-order helpers/actions. The five tests differ only
in three explicit `orderByAgent: {}` fixture seeds; two are byte-identical.
The save helper is byte-identical candidate/current main. Testing only candidate
would miss the actual version-gated regression. Before Agents RED, ROOT must
explicitly authorize integrating the exact reviewed base by a normal merge (or
specify an equally faithful bounded integration). Do not rebase for drift, copy
all main files, silently cherry-pick/revert profile-order work, or reconstruct
its store/handler contracts. Preserve scroll/Global edits while resolving only
real overlapping owned changes. If ROOT does not authorize integration, checkpoint
this concrete prerequisite rather than claiming a candidate-only RED.

## Scope

### In scope

- New-agent accepted publication provenance in the two existing save callbacks.
- Existing hook's explicit new-agent branch; current independent metadata/options,
  order, partial MCP result, draft IDs, shared Save and navigation.
- The ten exact mock/position/order/counter compatibility failures, preserving
  their original behavioral assertions and keeping profile-order behavior.

### Out of scope

Ordinary save guards, existing-owner resurrection, global ordering/version/store/
event redesign, APIs/backend/auth, general lifecycle/tombstone framework, new UI
copy/layout/interaction, dependencies/installs, broad tests/build/browser runs,
old-head CI reruns, or changes to already passing Global tests.

## Technical approach and exact production coverage

- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`: extend the existing
  callback creation context/type minimally and supply explicit new-agent provenance
  plus accepted profiles at both `saveNewAgent` upserts. Preserve draft/MCP helpers.
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.ts`: recognize
  that provenance, allowing the accepted new owner while retaining current
  independent options and existing new-profile-first projection. Preserve ordinary
  save delegation and existing-owner missing-target guard. No change to the
  current version helper, ordinary save-sync helper, setters or registered handlers.

Compatibility fixtures are named by the work order. Profile-id lookups preserve
exact normalized-value assertions; creation order assertions adopt current
new-first order explicitly; delete counter assertions expect exactly one existing
version bump while retaining all membership/metadata/choice assertions. Agents
page mock supplies stable store API/current state for its own new dependency,
preserving self-update refresh/error assertions and mocking existing profile-order
readiness only if the actual dependency requires it. No blanket mocks or skips.

## Mobile compatibility

This is state/data publication through the existing Agents page/shared Save and
existing task/subtask choices. No layout, scroll, overlay, touch, copy, navigation
or operator step changes. The nearest shipped phone Agents creation/selection
surfaces retain their controls; the mobile-parity narrow exception applies.
Real page/coordinator/API/store/registered event/picker tests prove shared data
behavior; no new mobile E2E or ASCII composition is needed. Reassess before any
actual interaction or viewport change. The scroll package's phone E2E remains separate.

## Tests

Reuse real `agent-create-catalogue.test.tsx` new-owner success and partial MCP
cases as independent causal RED against the actual integrated version-gated
source; require ordinary/new-owner no-event and rejected POST controls passing.
Then GREEN of all five failed suites, changed creation-sync/helper suites and
existing ordinary-save sync guard suite. Cover existing-owner absent after await,
ordinary-save absent/version mismatch, accepted new-agent identity once, newer
independent choices and partial MCP/navigation/draft remapping. Only external
boundaries isolated. Preserve all six creation ACs and adjacent existing-owner
requirements; no weakened assertions to hide lost accepted membership.

The previously completed creation/target/deletion packages remain historical
records. Record these necessary new CI results here; do not relabel their old
commands/counts as new evidence or add procedural polish.

## Work orders

- [ ] [Task 01: Restore new-agent publication and CI compatibility](task-01-restore-new-agent-publication.md)

## Verification results

DESIGN_READY, 2026-10-10. ROOT full actual-file review completed09:43; LATER
explicit Agents implementation/integration release remains required. Catalogue
validation369decisions/1513specifications and all spec lint passed. Link/reference
preflight passed: unchanged requirement/all6ACs/ONEorder/two actual existing
production paths covered/errors[],21local links resolved. Actual three Agents
docs are exempt; projected production coverage is separately covered. Full raw
inventory111180entries/status retained alongside pre-existing scroll edits, no
foreign Git changes. Initial preflight rejected19added/17removed ignored managed
scroll Vite/report paths; final preflight explicitly reconciled those authorized
task-owned build artifacts, hiding no path and deleting nothing for an audit.
No unknown addition/removal remains. Agents code/tests/integration/runtime remain
unmodified. ROOT reviewed faithful integration as normal merge of exact
02ff0578357040b0546ea17cd9a00dff9ca9ee3b after later release, preserving owned
scroll/docs; a normal-hooks local checkpoint commit/no push is allowed if dirty
overlap requires it. No stash/all-main-file copy/drift rebase. Actual fix paths
will be measured relative integrated main, with upstream merge recorded distinctly.
No blind retry or premature publication.

## Risks

- Conflating new-agent create with an absent ordinary/existing owner would restore
  removed owners; callback provenance must remain operation-specific.
- Candidate-only green can hide the tested-merge regression; faithful reviewed
  integration must precede RED, with full native/source receipts.
- Preserve base profile ordering and unrelated options; do not use full-list flatten
  reconstruction or stale indexes to repair the tests.


## Released implementation checkpoint

ROOT later implementation/integration release and exclusive GLOBALheavy111 received.
Normal-hook local scroll checkpoint `dcb0137d075f3826fe5499a62f751a5a020f8f21`
then normal merge `232f6a98699965c26403249844f0c70457e64535` integrated the
exact prerequisite `02ff0578357040b0546ea17cd9a00dff9ca9ee3b` without conflicts.
Full candidate-to-prerequisite dependency manifests and lockfile comparison was
empty; retained dependencies were used without install. Upstream integration is
separate from the owned correction measured relative to that exact prerequisite.

Before production, original causal RED native79510 actual JOIN1/b24e12 showed
precisely the two missing accepted new-agent publications; ordinary profile,
new-agent no-event, and rejection controls passed (three). A preceding attempt
retained an unrelated dirty-state assertion in the new control, corrected before
qualification. Partial loss now reports a precise missing-owner assertion.

Only the two named production paths changed: `newAgent: true` in the two accepted
new-agent callbacks and the private existing creation-publication branch. Existing
owner reconciliation, ordinary-save/version/missing-owner guards remain intact.
Ten proved stale CI fixtures now assert accepted-new-first order, metadata by ID,
one deletion version advance, and the stable current-store API. Agents page uses
the real ToastProvider required by the integrated ordering hook.

Affected eight-suite GREEN native40964 actual JOIN1/b10de1: 90 passed, two page
fixtures failed only for the missing ToastProvider. Corrected page-only native21141
actual JOIN0/e00b8b: two passed. All 92 affected cases now pass; seven passing suites
were not replayed. Scoped lint/staged i18n/docs/normal hooks/publication pending.
State/data-only mobile exception retained; no copy/layout/navigation/browser changes.


### Final local checks

Scoped lint native11999 actual JOIN0/6a758b passed after extracting the repeated
saved-route test constant; first lint's single duplicate-string warning is retained.
Staged i18n check and exact-base ratchet native4326 actual JOIN0/bb20a2 passed,
zero added/four modified production files clean. Catalogue (370 decisions/1519
specifications), full spec lint and whitespace passed. Complete unfiltered
tracked/cached/untracked inventory retained (111259 entries); actual 23 owned
paths relative exact02ff expose all four production paths with three separately
owned work orders and coverage errors[]. Documentation exemptions are recorded
separately from production coverage. The first aggregate coverage invocation
omitted the two unchanged reused requirements from its input map; corrected
complete inputs passed without modifying those requirements.

Normal hooks/one batched publication and fresh hosted current-head acceptance remain.
