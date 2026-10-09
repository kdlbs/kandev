---
created: 2026-10-09
status: implemented
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
legacy_specs: []
---

# Implementation plan: Agent creation catalogue preservation

## Overview

Publish normal creation results over the current owning-store catalogue so a
different configured owner's live profile remains available in the actual
picker. One sequential work order independently adds the real page regression,
makes the bounded publication correction, then checks its compatibility.

The design turn ended with four unstaged/uncommitted artifacts. ROOT fully
reviewed them and released execution at 2026-10-09 03:07 UTC in this original
primary session, with exclusive GLOBAL LOCAL-HEAVY lease. Receipt:
`/tmp/kandev-root-child92-design-review-release-20261009.json`. Merge authority
remains NONE. Vitest/ESLint/typecheck use Node 4 GiB, Vitest one worker; all
heavy commands run serially with original handles, terminal joins and fresh
owned-group absence. Changed i18n:check and documentation coverage are added
to the exact work-order checks before execution.

## Inputs and ownership

- [Requirements](../../specs/agents/requirements/creation-catalogue.md),
  `REQ-AGENTS-CREATION-CATALOGUE-001`, criteria .1 through .6.
- [Design](../../specs/agents/system-design/creation-catalogue.md).
- Agents owns configured profiles and local selectable projection. Existing
  layout requirements only define creation entry points; portable settings,
  appearance and executor editor packages do not own this contract.
- Accepted ROOT evidence is the two receipt/brief files under
  `/tmp/kandev-root-agent-create-catalogue-discovery-20261009/`.
  Corrected native20995/3ff8ce actually joined exit1 has one causal failure,
  two controls and both actual store/picker observations. Earlier92011/146341
  is retained as a noncausal fixture matcher failure. Do not replay either.
- Initial checkout and local main equal
  `8ca57f611c90ee696043b340883ac8b02aab0ad2`; all three source blobs match the
  receipt, as recorded in the design. Initial git status was clean.

## Scope

Own `useAgentStoreSync` in
`apps/web/app/settings/agents/[agentId]/page.tsx:169`, its normal existing-owner
and discovered-owner creation callbacks, and independently authored integration
coverage. The cause is its captured `settingsAgents` closure after an await.
Read `useAppStoreApi().getState().settingsAgents.items` inside the publication
callback. Retain accepted target assembly and existing projection/actions.

Exclude backend/schema/transport changes, caches, timestamp policies, store
refactors, other editors and other writers. A different existing owner is the
qualified concurrency boundary. New same-owner concurrency claims need their
own causal evidence and ROOT scope qualification before any implementation.

## Compatibility inventory

| Flow / actual resource | Publication / expected behavior | Evidence |
| --- | --- | --- |
| Existing owner, POST `/api/v1/agents/<id>/profiles` | Page callback replaces assembled accepted target over current other owners. Existing target profiles retained. | T1, T2, T3, T4 below |
| Discovered owner, POST `/api/v1/agents` | Same callback appends accepted owner; following MCP saves and optional MCP-path PATCH remain. | T5, T6 |
| Profile MCP `/api/v1/agent-profiles/<id>/mcp-config` | Partial accepted identities publish before original MCP error propagates; action at `app/actions/agents.ts:295`. | T4, T6 and current helper coverage |
| `agent.profile.created` for another existing owner | Real handler publishes normalized profile to catalogue and option. | T1, T3, T4, T5, T6 |
| Selected independent choice in `AgentProfilePicker` | Keep enabled option, label and ability to select after completion. | T1 through T6 as applicable |
| Missing owner / orphan option | Current WS path creates only a flattened option; creation flatten can drop it. No orphan-retention guarantee added. | Source limitation, not claimed fixed |
| `CliProfileEditor.saveNewProfile` | Calls real create actions and forwards `onSaved(profile)`; does not use page upsert. | Inventory only; excluded |

No rendered layout or copy changes. Under the mobile-parity pure state/data
exception, the unchanged page/picker composition shares this publication on
phone and desktop. No ASCII UI preview is needed for this nonvisual change.

## Tests

All new methods below belong in
`apps/web/app/settings/agents/[agentId]/agent-create-catalogue.test.tsx` and
must be authored independently after release. No reading/copying/importing
ROOT's protected proof, including via a helper or symlink.

Use actual Page, providers, store, coordinator/saveAll, API actions/normalizers,
WS handler, routing and picker. Stub only external fetchJson transport and
external editor capability. Test strings stay in recognized `.test.tsx` or
test-owned helper parameters; no copy-scan bypass or production test seam.

| Test name | AC suffix | Stimulus and required observations |
| --- | --- | --- |
| T1 `retains a live different-owner profile after accepted profile creation` | .1, .2, .6 | Real `/settings/agents/claude-code?mode=create`, edit Name, saveAll, await intercepted POST admission; real handler publishes independent profile to another seeded owner, open picker and select it; release accepted older response. Assert target identity/configuration and both current catalogue plus selected real label/enabled choice, no Unavailable entry. |
| T2 `accepts profile creation without an intervening event` | .2, .6 | Baseline creation succeeds; accepted ID exactly once, existing target profiles remain, payload contains edited fields, coordinator settles and actual route replacement occurs. |
| T3 `rejects creation while preserving the live choice and newer draft` | .3, .6 | Held POST, independent real event/selection, edit Name again during wait, reject POST. Assert no accepted target, live profile/actual choice retained, newer Name, dirty/failed coordinator and unchanged create route. |
| T4 `publishes partial accepted profile creation over the current catalogue` | .1, .4, .6 | Additional-profile POST accepts, following real MCP action rejects after independent event. Assert both catalogues and picker retain independent profile; accepted target persisted ID and pending MCP draft, original error/shared failure and no successful navigation. |
| T5 `publishes a newly created agent over the current catalogue` | .5, .6 | Discovered agent absent from catalogue, hold owner POST, independent existing-owner event/selection, accept owner/profiles. Assert appended canonical identities, independent store/choice and actual navigation. |
| T6 `publishes a new agent partial MCP result without dropping a live choice` | .5, .6 | New owner POST succeeds, following MCP save fails after live event. Observe actual callback publication of accepted IDs/pending MCP data and independent store/choice; shared save failure plus current branch route replacement. Do not force this route to stay mounted. |

Assert the causal order through deferred request admission and actual WS/picker
state before releasing it; no sleeps or synthetic page callback. Each test
settles its original save promise and deferred transports in cleanup. T1 is
the principal causal RED. T4 through T6 test actual callback compatibility,
not a new same-owner resolution claim. Any unexpected boundary failure is a
ROOT checkpoint before expanding scope.

Current helper tests at `agent-save-helpers.test.ts:304` and `:353` cover
MCP partial reconciliation and retries but mock actions/upsert. Draft identity
tests at `:255` and `:280` cover `mergeSavedAgentDraft`. Provider helper tests
cover payload omission/defaults. They cannot substitute for the new actual
Page/current-store/picker tests. Existing dynamic save concurrency tests provide
a positive real-provider/transport-boundary pattern, not copied evidence.

## E2E evidence and mobile

The six permanent browser component integrations cover the complete local
edit/request/event/acceptance/picker chain with real product boundaries; they
are not a hosted Playwright or real-backend persistence claim. No new Playwright
file is scheduled for this bounded state-only repair. Mobile parity uses the
documented narrow exception because composition, touch, scrolling and navigation
rules remain unchanged. If that ceases to hold, checkpoint ROOT before choosing
broader tests or UI changes. Existing E2E CI parents still must succeed for delivery.

## Work orders

- [x] [Task 01: Preserve current catalogue during creation](task-01-preserve-current-catalogue.md)
  (`done`, sequential; no delegation).

## Verification results

Design validation completed on 2026-10-09: catalog validates 365 decisions and
1480 specifications; both new agent documents are discoverable; all specification
files pass; the specification-linter self-tests pass (36 tests). Source anchors,
API resources and test project selection were checked against current code.
Whitespace and relative Markdown links for all four untracked artifacts pass.
All light native commands returned terminal results. Production and permanent-test
checks were not run during design. After ROOT's later release, Task 01 proved
the independent causal RED and controls, then all six real-flow cases plus
focused helpers passed (48 tests). Final fixture-only changes passed all six
affected cases, ESLint and typecheck. i18n ratchet/check and actual changed-path
documentation coverage passed. Requirement/design are active/current, this
manifest implemented and the sole work order done. Exact handles, original
fixture failures and serial join/group receipts are in the work-order results.
Hosted review, CI and merge delivery remain subject to ROOT's separate gates.

Initially `apps/node_modules` and `apps/web/node_modules` were absent. After
release, exactly one pinned pnpm9.15.9 frozen install from `apps/` passed.
Existing Node24
mise binary is `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`;
pnpm9.15.9 executable is
`/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm`.
Use Bash with loginfalse. No runtime/config/cache hacks.

## Docs impact

Internal requirements/design/plan only. Public agent-profile docs, root README
and screenshot catalogue were checked for relevant creation/profile wording.
No public commands, labels, screenshots or creation procedure change. No new
ADR or system README change is needed: established agent ownership is retained.

## Authority and delivery checkpoints

- Task `415fc44b-d8f4-44b1-ba0b-a0c5adf9a3f1`, original primary session
  `277b8fe1-559b-49af-9b1a-2f8833aa3850`, ROOT child92. Design ends when the
  four artifacts and light validation are ready. No approval/model-switch
  question. ROOT direct-reads plan/conversation; callback queue fullness is
  never a gate and must not be retried.
- No agents/tasks/sessions/tabs/delegates/model switches. No heavy execution
  until later explicit ROOT implementation INTERRUPT and GLOBAL lease. One
  lease covers install/tests/lint/typecheck/hooks at a time. Keep every original
  native handle, actually JOIN terminal, prove exact owned groups gone before
  RETURN. Resource/timeout/transport/unknown/out-of-scope issues checkpoint ROOT
  before alternatives, retries, cache wipes or foreign kills. Normal active
  hooks; no bypass and no broad passing replay.
- Preserve ROOT protected candidate.test.tsx mode400 and SHA256
  `4d0998f479234c5a8e554eba1919753899dd8b3b395506dd58ee82e9f621d5d4`:
  never read/copy/import/replay/edit/chmod/remove. Never touch paused task
  `a6032d95-cc1e-4db8-adca-5b643ae4138d`, `serialize-workspace_e3isbq2j`, or
  volume `2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.
  Preserve managed worktree/deps/shared caches/foreign processes.
- Later ready PR must be clean/pushed/frozen except real corrective findings;
  canonical task repository association complete/errors[] and all five task
  auto-mutation flags FALSE, no relink of already-correct readback. Preserve
  PR template/live body and verify readback.
- After ROOT qualifies local-heavy RETURN, ONE attached all-terminal90m
  observer, GNU91m kill10 cadence60. Preserve its original native handle across
  crashes. No duplicate observer/timer/replacement before actual END/join/gone
  and ROOT authorization. One first bounded WHILECI semantic inspection only on
  ROOT release. Accept configured authenticated CodeRabbit App347564 FULL
  CURRENT ALLFILE/source=covered/kind=reviewed substantive automatic report
  with zero requests when sufficient; at most one necessary real-gap request
  after inspection, never treat ACK/progress as evidence. Ground findings;
  defer optional polish/head churn.
- Delivery requires fresh exact-head six required contexts plus actual
  Backend/Frontend/E2E parent SUCCESS, errors[] and zero hidden/actionable/human
  findings. No blind/whole/passing hosted reruns. Only ROOT-budgeted audited
  failed job plus dependencies after parentterminal/currentOPEN/frozenhead;
  budget keyed workflow+jobname, not IDs.
- Merge authority NONE until separate ROOT serial static-compatibility grant.
  Normal expected-head squash only: no admin bypass/rebase/synthetic merged
  tests. Verify actual merge SHA/tree/parent/all owned blobs/remote main
  inclusion and every owned handle joined END. ROOT alone independently FFs,
  archivesABSENT, releases proof and refills. Record actual receipts.

## Risks

- A store read at admission instead of callback publication repeats the defect.
- Replacing helper target assembly with a profile-level merge changes intentional
  create-mode and partial-result behavior without same-owner evidence.
- Tests that mock the page/save coordinator/API actions/routing/picker conceal
  the stale closure or fabricate navigation outcomes.
- New-agent MCP errors intentionally navigate after publishing a partial result;
  an asserted all-or-nothing rollback would be a different contract.
