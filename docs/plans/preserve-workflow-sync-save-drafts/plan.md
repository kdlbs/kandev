---
created: 2026-10-09
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002
system_design:
  - ../../specs/integrations/system-design/workflow-sync-settings-lifetime.md
legacy_specs: []
---

# Implementation Plan: Preserve workflow source drafts during Save

## Overview and design checkpoint

Preserve a newer complete raw workflow-source draft while recording an older
accepted Save. Keep the actual configuration dialog open for that draft while
ordinary unchanged successful Save still normalizes and closes. Deliver this
local acknowledgement and its real consumer tests in ONE sequential work order.

The completed design turn left all four artifacts unstaged and uncommitted.
ROOT reviewed their exact hashes and sent a later same-primary implementation
release. CHILD90 returned localheavy and is hosted-only; CHILD91 now holds the
exclusive global local-heavy lease. Task01 implementation and scoped verification are complete. Same primary, no
operator approval/delegation/new task/session/tab/model switch. Normal ready-PR
delivery is authorized; merge authority remains NONE pending a separate ROOT
serial grant. The version-safe task plan records current identity/recovery/gates.

## Ownership, evidence and assumptions

Integrations owns provider-aware source configuration and its settings outcome.
Extend the [existing requirement](../../specs/integrations/requirements/gitlab-workflow-sync.md)
with REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003 and extend its existing
[technical design](../../specs/integrations/system-design/workflow-sync-settings-lifetime.md#save-draft-acknowledgement).
AC002.8 explicitly excluded edits during same-workspace Save; this is a new
bounded contract, not an accusation against the earlier lifetime correction.
AC002.5 continues to require truthful accepted-write booleans.

Accepted ROOT proof: original 40dfbe/session26965/actual792627 exit1, one causal
failure reaching BOTH soft assertions (newer Branch lost, actual dialog closed)
and TWO passing controls (unchanged success closes, current failure preserves).
Only `qualified-proof.json` and `future-design-brief.md` were read from
`/tmp/kandev-root-workflow-save-draft-discovery-20261008/`. Proof uses real
Section/Dialog/hook, StateProvider/createAppStore, ToastProvider and real API;
only external fetchJson mocked. Originals joined and owned groups were absent.
Proof base: `5aa06bcefe02b6ce7b7c321f7092efe89f0c8992`. Current checkout and
remote main: `a06b7dad4e6f9a911e203188e6a734193ea7f776`. Hook/dialog/section blobs
are identical at both bases (`3c6d77fe`, `1ddfce39`, `f54fcde8` respectively).
This accepted proof is not replayed and does not count as permanent regression
RED. ROOT's protected candidate (0400, SHA256
`3846050d361ade95ad2a3c162be3af0a3ec16de712eaf91fc186df5db191d2b2`) is never read,
copied, imported, replayed, edited, chmodded or removed.

Confirmed choices: full raw value equality including displayed URL; canonical
config always recorded for current success; conditional editable adoption and
dismissal; truthful boolean; retained draft usable for next Save. Source audit
verifies that Section is the sole production hook consumer and Dialog the only
save caller; Workflow Sync has no settings-save contributor. No material
unresolved assumption remains. The nearby Office config acknowledgement is an
example of conditional editable adoption, but its form-only comparison cannot
cover this hook's raw URL or actual dialog-close boundary.

## Scope

### In scope

- Local immutable full draft/latest snapshot and raw submitted acknowledgement
  in `apps/web/hooks/domains/settings/use-workflow-sync.ts`.
- Immediate save dismissal admission in
  `apps/web/components/settings/workflow-sync-dialog.tsx`; Section plumbing only
  if the actual caller boundary requires it.
- Independent real hook and Section/Dialog/provider regressions with
  transport-only mocks, scoped compatibility controls and documentation results.

### Out of scope

Global framework/coordinator, shared revision or concurrency/ordering policy,
backend/API/schema/migration/auth/provider/poller changes, transport abort,
persistent drafts, runtime flags, layout/copy/touch/scroll/breakpoints, browser,
build, broad local suites, runtime harness/cache edits and optional polish.
Earlier GitLab provider and lifetime packages stay complete and historical;
their results do not become this extension's results. ROOT owns proof release.

## Technical approach

Follow the design's pre-normalization raw comparison. One local draft holds all
eight form fields and displayed URL. Admitted edits/reset publish a pure next
draft and latest private snapshot together outside render/updaters. Save retains
its captured request shape, always records current accepted config, and adopts
canonical draft only if every raw value still matches submitted. Use the small
optional read-only `onDraftAccepted` controller notification before reset;
Dialog's invocation-local admission flag combines with truthful success and
the existing open/controller lifetime before dismissal. No-argument
`handleSave()` continues to resolve boolean. Presentation handling cannot change
a successful transport outcome to false. Keep removal's target-generation guard
and pending tickets independent. A post-await draft comparison would incorrectly
reject canonical normalization; comparing only parsed fields would lose invalid
or equivalently parsed raw URLs.

| Provider / transport | Source identity | Required outcome | Evidence / unsupported case |
| --- | --- | --- | --- |
| GitHub / existing fetchJson API | workspace + owner/repo | Full raw preservation; unchanged trim/canonical adoption/close | New real draft suites plus existing URL/API/current controls |
| GitLab / same API | workspace + project_path; existing backend connection owns host | Provider switch and raw path/link preservation; unchanged canonical close | New actual dialog provider switch and hook raw matrix plus GitLab URL controls |
| Other provider | Unsupported existing type/API | No new support or fallback | No new coverage claim |

## Tests

All new tests are independently authored AFTER release in recognized `.test.tsx`
files, using real `WorkflowSyncSection`, `WorkflowSyncDialog`, `useWorkflowSync`,
`StateProvider`/`createAppStore`, `ToastProvider` and API clients. Only external
fetchJson is mocked. No internal form/store/lifetime/coordinator/reporter mocking.
Test-owned UI harnesses control the real open state; deterministic deferred
transport owns pending/resolve/reject, with no sleeps. Drain owned work at cleanup.

| Criteria | Planned suite and named outcome |
| --- | --- |
| 003.1-.2, 002.5 | `hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx`: accepted canonical config and true outcome coexist with complete newer raw draft |
| 003.2 | Same hook suite: each form scalar/identifier, provider, valid/invalid/same-target raw URL; full draft unchanged after ACK; multiple edits in one batch |
| 003.2, .6-.7 | `components/settings/workflow-sync-section.save-drafts.test.tsx`: newer Branch remains and actual dialog stays open at 390/1024; path/provider/link/poll/interval cases |
| 003.3 | Both new suites: unchanged normalized success adopts canonical form and displayed URL, then closes; first Save/provider-switch success; edit-away-and-back to submitted values |
| 003.4 | Both new suites: return to pre-Save baseline while submitted branch differs, preserve reverted draft; next POST uses retained draft and closes on unchanged success |
| 003.5 | Both new suites: current rejection keeps complete newer draft and open dialog; saving clears; invalid retained URL blocks Save until correction; retry submits corrected current values |
| 003.6, 002.1-.4, .7 | Existing real hook/section lifetime suites: retired workspace, independent instances, StrictMode, close/reopen and pending ownership; maintain truthful old outcomes |
| 003.7, 002.6, .8 | Existing hook URL redisplay/provider/reset, API and GitHub/GitLab parser suites; existing removal/Sync-now/initial/background controls |

The work order owns exact commands and `@covers` traceability. Existing dialog
unit mocks are compatibility controls only, never the new causal evidence.
Existing settings-save provider/revision suites remain adjacent contributor
controls; do not add a contributor to Workflow Sync or change Office settings.

## E2E and mobile assessment

End-to-end evidence for this bounded data transition is the real Section to
Dialog to hook to API transport component regression. The shipped desktop and
phone composition is unchanged, including existing mobile removal confirmation.
Apply mobile-parity's pure state/data exception: branch preservation and ordinary
closing at 390/1024, plus raw hook coverage. No layout preview, new mobile
Playwright, browser/build or geometry promise is needed. If composition changes
or a new browser-specific cause appears, checkpoint ROOT before broadening.

## Public documentation audit

`docs/public/workflow-sync.md` is a how-to with stored-field/API reference.
Configure/Save/Sync-now, polling and Delete guidance remains correct; no invented
user recovery step or public-copy edit. Searches of root README and
`docs/screenshots.md` reveal no pending-save claim to change. Owning requirement
and design record the new behavior. Root/scoped AGENTS conventions stay accurate.
Actual implementation retains the audited guidance and unchanged UI composition.

## Work orders

- [x] [Task 01: Preserve full raw Save drafts and current-dialog admission](task-01-preserve-save-drafts.md) (done, sequential; no dependencies).

## Verification results

Design validation complete (2026-10-09):

- `python3 scripts/list-docs.py validate`: PASS, 365 decisions/1,478 specifications.
- `python3 scripts/lint-spec-files.py --all`: PASS. The initial size failure
  was corrected by tightening only new requirement prose; final owner file
  20,082 bytes, below 20,480. No exception or existing migrated-detail rewrite.
- Actual-path `validateCoverage` preflight: PASS with `errors: []`,
  `requiresCoverage: false`, four actual documentation paths exempt. This is
  not implementation coverage; independently checked one order, both REQs,
  all 15 ACs, design/manifest membership and 18 existing local links: PASS.
- `git diff --check` and scoped manifest/order status: PASS; exactly the four
  package artifacts are modified/untracked, none staged. No production or
  permanent test diff.

Initial bare `node` preflight could not start (exit127, no coverage verdict).
The existing `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node` then ran the light
preflight/link checks successfully; no install, shell config, harness or cache
change. ROOT checkpoint delivery returned queue_full; no retry and no approval
inference. This durable package/live task plan supplies the direct-read recovery
checkpoint. That design checkpoint preceded the later reviewed release. Accepted ROOT proof
is not a local test run; independently authored implementation evidence follows.

Implementation validation complete (2026-10-09), after ROOT's reviewed release:

- Independently authored transport-only real hook/Section/Dialog/provider RED:
  native65715 exit1 (23 failures/9 passes); affected fixture RED native23094
  exit1 reaches both newer-branch/dismissal soft assertions at 390/1024.
- Minimal raw-draft acknowledgement preserves accepted config/true/feedback;
  Dialog admits closing before canonical adoption. Section composition unchanged.
- Scoped matrix: original native81525 had 159/161 pass; two causal fixture/old
  excluded-policy expectations were corrected and affected native35561 passed
  44/44. The unchanged nine suites supply the other 117 passing cases, totaling
  161/161, including 33 new regressions. This is combined evidence, not a claim
  that the initial full command passed. Private completion-helper lint extraction
  then passed its affected dialog suites 43/43 (native57394).
- Actual six changed frontend files formatted; scoped eslint is clear, including
  affected zero-warning repair (native6395). Typecheck native35307, i18n:check
  native72006, i18n:ratchet native29458 and final prettier --check all exit0.
- Every original above reached actual native terminal/join and fresh owned
  wrapper/child groups absent. Receipts/logs are task-owned
  `/tmp/kandev-child91-<label>.json` / `.log`; no detached verdict.
- Conditional missing-worktree dependency install used existing pnpm9.15.9 with
  frozen lockfile (native33833 exit0). No cache/runtime harness mutation.
- Final catalog/spec/links/actual implementation documentation coverage and
  whitespace checks recorded in the live task checkpoint. Normal active hooks
  and hosted delivery remain required; no commit/PR/merge success is implied here.
- Public guide audit needs no edit. Mobile state/data exception retains identical
  composition; real 390/1024 shared-path tests cover outcomes. No browser, build,
  full local suite, proof replay or permanent test copied from ROOT.

## Risks and delivery gates

Raw URL can differ while parsed identifiers match. Canonical normalization can
change an unchanged submitted draft. Current config can differ from retained
draft. A stale render, a ref mutation during render/updater or a payload-only
comparison would lose these distinctions. Value equality must permit edits back
to submitted values while preserving edits back to old config. Own successful
config changes must not trigger removal-generation-based Save rejection.

Stay in task `cc2f9b51-a441-478a-8e20-ccd7d98f0800` / primary
`714e8fb0-9581-4351-b013-5740766dc906`. After explicit implementation release and
lease, run one original heavy command at a time, retain native handles, join
actual terminal results and prove fresh owned groups gone. Resource, timeout,
transport, unknown or out-of-scope failures checkpoint ROOT before alternatives,
duplicate commands, cache wipes, foreign kills or weaker checks. Conditional
frozen install uses existing pnpm9.15.9 only after lease. Preserve foreign
processes/worktrees/caches/archives, paused oversized dirty worktree and the
forbidden unproved Docker volume identified in the live task plan.

After causal RED/minimal GREEN, exact checks and normal hooks, publish a ready
PR on a frozen head. No main-only rebase, passing-proof replay, optional polish,
blind hosted retry or duplicate review request. heavyRETURN requires actual
all joins and fresh owned groups absent before ONE original 90-minute
all-terminal observer under GNU timeout91m/kill10/cadence60. Retain its handle
across crashes/changes; no duplicate timer/pr-state/manual-review/recursive
observer. Actual END/join/gone precedes any ROOT-authorized replacement; timeout
or lost transport is NO VERDICT.

All six actual required contexts and actual Backend/Frontend/E2E parents must
succeed at the exact head, with fresh complete errors[] and zero visible,
hidden, actionable or human findings. Authenticated configured CodeRabbit
App347564 must substantively review FULL CURRENT ALL changed files with
source=covered/kind=reviewed. Sufficient automatic coverage needs zero requests;
only one necessary real-gap request after inspection. Ground every disposition.

Separate ROOT SERIAL MERGE grant remains mandatory after actual hosted-ready
END/all joins. Then normal expected-head squash; verify actual SHA/tree/parent,
all owned blobs, remote main inclusion and all owned joins END. ROOT owns
independent FF/archive ABSENT/proof release/refill. Completion requires actual
merge and closure, not design or PR readiness. Implementation followed ROOT's later explicit release; this manifest does not grant merge authority.
