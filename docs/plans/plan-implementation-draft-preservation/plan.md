---
created: 2026-10-08
status: in_progress
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Implementation Plan: Preserve Newer Drafts When Starting Plan Implementation

## Overview

Apply the shared composer's existing accepted-payload clearing contract to both
implementation action hooks. Deliver independently authored rendered regressions
and the two-hook correction in one sequential bounded work order. This package
is design only until a later explicit implementation instruction in the same
user-started primary session and a ROOT local-heavy lease.

## Ownership and settled assumptions

UI owns reusable composer reply admission and browser-local drafts through
`REQ-UI-SESSION-REFRESH-EFFICIENCY-004`, particularly `.3`–`.6`; Tasks keeps
delivery, session lifecycle and attachment claims. Reuse that owning
[requirement](../../specs/ui/requirements/session-refresh-efficiency.md) and
[design](../../specs/ui/system-design/session-refresh-efficiency.md).
The prior [composer settlement package](../composer-draft-settlement-ownership/plan.md)
is completed; its visit helper and accepted-payload policy are dependencies,
not work to reopen. The original
[session refresh package](../session-refresh-efficiency/plan.md) remains
in progress and Task 05's editor-performance investigation remains open.

Confirmed by the caller: preserve newer unsent work, successful unchanged
clearing and failed-send retries; scope the two existing hooks and immediate
glue only if causal. Verified in source: both paths clear via a mutable ref
after awaits, fresh also deletes old-session storage with no ref, and the
shared composer already exposes visit-bound `clearAcceptedPayload`.
There are no unresolved material product choices. Reusing existing ownership
and payload matching is a routine conformance fix; no new ADR is warranted.

## Evidence and root cause

Base: `6c21e0ce22406c39bd74ccb7123083ca273a6ec4`. ROOT receipt:
`/tmp/kandev-root-plan-implementation-draft-discovery-20261008/qualified-proof.json`.
Protected mode-0400 evidence `candidate.test.tsx` in that directory has SHA256
`a920fab002027f33eb155c7fcce614b29f6d7470460366719f1d12bcbb3c73fe`.
Inspected once, read-only. Never copy, import, replay, edit, chmod, release or
delete it. The proof used actual `ChatInputContainer`, TipTap, the runner,
StateProvider/store, ToastProvider and TooltipProvider; only external WS/fetch
transport was mocked. Original native start `1fbf51`, session `67394`, terminal
`d07884`, exit 1, joined and owned groups gone per ROOT receipt.

Original instruction is submitted through Implement, newer `NEXT` is displayed
and saved while `message.add` is held, then accepting the original request makes
the actual editor empty. One causal assertion fails; unchanged accepted clear
and rejected-send preservation controls pass. No setup/import failure or
unhandled errors. The subsequent saved-content equality assertion was not
reached: this is live-editor-loss evidence, not an independent persisted-loss
RED. It is a native DOM adapter, not a full ChatInputArea/phone/browser proof.
Do not rerun it or claim that the fresh path was experimentally reproduced.

`useImplementPlan` recollects no submitted owner at settlement: after message
acceptance and marker await it calls `chatInputRef.current?.clear()` and directly
sets saved content to null. `useImplementFresh` repeats the pattern after launch,
primary assignment, marker and active-session change, including an unconditional
old-session storage deletion without a composer ref. These paths bypass the
already delivered visit and accepted-payload guards.

## Caller inventory and technical approach

| Entry point | Actual path | Bounded correction/evidence |
| --- | --- | --- |
| Desktop composer Implement | ChatInputArea -> usePlanActions -> useComposerProps -> ChatInputContainer/body -> ChatInputToolbarDesktop -> ImplementPlanButton -> runner(false) | Capture original callback/payload, deferred actual editor/storage regression |
| Phone composer Implement | Same shared composition -> ChatInputToolbarMobile -> button(false) | Same data rule; rendered phone component case |
| Composer fresh option | Same toolbar dropdown -> runner(true) -> useImplementFresh -> real launchSession -> external session.launch | Deferred launch/secondary acknowledgements, original and destination draft controls |
| Plan toolbar | PlanPanelHeader -> runner, no composer ref; saves unsaved plan before action | No draft-clearing authority for either branch; retain marker/save behavior |
| Next work step | usePlanActions implementPlanHandler -> proceed() if nextStepIsWorkStep | No raw composer clear; preserve routing/policy, existing use-plan-actions controls |

Capture one original handle and its `clearAcceptedPayload` function along with
the raw composer markdown and attachment descriptors before any await. Use the
same captured input to build the outgoing request. After acceptance call that
captured function with `{ message: userText, attachments }`; never use the
augmented agent prompt for draft comparison. Remove both direct storage clears
and mutable-ref raw clears. Optional missing callback fails closed for clearing,
while preserving the operation's existing success result and side effects.
Do not add a generic snapshot/coordinator/lifetime framework or extend the
public handle. No production glue change is currently indicated.

The existing accepted-payload path preserves the entire draft if text or
attachment matching fails. Ordinary send's different attachment-change policy
remains untouched. Formatting that changes serialized markdown is a new payload;
preserve its rich JSON. Identical matching markdown follows the existing trimmed
comparison, not a new edit-generation policy. Captured callback admission handles
session switches, return visits, replacement, unmount and null-session ownership.
Do not move fresh-session activation or clear retired-session storage manually
to force historical cleanup.

## Scope

### In scope

- Two-hook correction and removal of their competing clearing/storage writes.
- Real rendered shared composer/action composition, direct persisted-content and
  remount assertions, matching/rejected/attachment/lifecycle controls.
- Narrow updates to the two isolated runner test fixtures and expectations to
  represent accepted-payload clearing; retain their transport/side-effect checks.
- Owning requirement/design clarification and this package's delivery records.

### Out of scope

- Backend, schema, API, global drafts, generic coordinator, new dependency/flag.
- Transport/timeout/replay/exactly-once guarantees, navigation/runner/start policy,
  plan-mode routing, context snapshot cleanup, broader upload lifecycle.
- Copy, layout, touch targets, breakpoints, screenshots or browser/build/full-suite
  verification. No native agents or new persistent tasks/sessions/tabs, worktrees,
  model/profile switches or foreign resource cleanup.

## Tests

New independent suites:
`apps/web/hooks/domains/kanban/use-plan-implementation-draft.test.tsx` and
`apps/web/hooks/domains/kanban/use-implement-fresh-draft.test.tsx`; optionally one
local `plan-implementation-draft.test-helpers.tsx` for real provider/fixture
composition. Never reuse the protected candidate. No mocks of internal hooks,
clear methods, editor, storage, providers, stores or launchSession/plan API.

| Acceptance | Required real-composer evidence |
| --- | --- |
| `004.5` | Same-session newer plain text AND newer formatted markdown survive live editor, saved text/rich JSON and remount; test saved values directly even if another assertion fails |
| `004.5` | Unchanged accepted text/attachments clear through shared API; rejected delivery preserves and permits retry; marker failure after successful delivery retains success |
| `004.4`, `004.6` | Held original action, committed A-B-A, same-session keyed replacement, task/session change or unmount; equal text in successor must survive live/storage/remount |
| `004.5` | Added/replaced attachment or changed delivery mode while pending preserves current descriptor/storage and draft; original outgoing snapshot remains correct |
| `004.4`–`.6` | Fresh launch plus late primary/marker acknowledgements; new draft in original visit survives until existing activation; reopened original and pre-existing destination remain intact |
| `004.5`, `004.6` | Actual plan-toolbar no-ref same/fresh paths with separately mounted/saved composer: draft untouched, normal prompt/save/marker/activation unchanged |
| `004.3`–`.6` | First-party desktop and phone action wiring exercised with real production providers, store and editor; trigger Implement/fresh through actual controls |

Use externally deferred WS requests and allowlisted fetch fixtures. Record exact
payload IDs, task/session, UUID presence, plan_mode, attachment/context descriptors,
timeouts and expected successful marker/primary/plan-mode side effects. Avoid
tautological guard-count tests. Retain existing readiness and workflow branch
controls in the listed suites. Rejected fresh launch or missing session ID must
not clear; retain existing guards rather than changing launch response policy.

## Mobile parity and E2E assessment

This is purely state/data settlement in existing components: no rendered markup,
touch, scroll, focus, navigation, safe-area or viewport behavior changes. Existing
ChatInputToolbarMobile supplies the phone entry and shared Implement split button.
Use the mobile-parity skill's explicit state/data-only exception, as in the prior
ownership package. Real rendered component tests at desktop 1280 px and phone
390 px verify action wiring and drafts; they are not full browser or visual proof.
No new Playwright run/build is authorized. No ASCII UI preview is needed because
composition is unchanged. If implementation requires presentation or navigation
changes, checkpoint ROOT before expanding scope.

## Public documentation assessment

The docs-maintainer audit found existing attachment readiness guidance in
`docs/public/tasks-and-workflows.md` and plan workflow media. This repair restores
composer preservation without new labels, commands, config, navigation or visual
changes. The actual implementation audit adds one short draft-preservation paragraph
to the existing task-plan how-to in `docs/public/tasks-and-workflows.md`; no new
public page or screenshots are needed. The state/data-only mobile exception applies
with real desktop1280/phone390 action and persistence coverage.

## Work orders

- [ ] [Task 01: Use captured accepted-payload clearing](task-01-capture-accepted-plan-payload.md)
  (in progress: local implementation and qualified RED/GREEN recorded; final scoped
  checks and normal publication/hosted review/merge gates remain).

Dependencies: existing delivered shared composer API only. Execute sequentially
in the same primary; design artifacts are not implementation authorization.

## Operational identity and phase barriers

Preserve `<kandev-system>` instructions, task
`259783f3-f259-4739-8952-5f36206a99a2`, session
`eccb766a-d286-4111-a74e-b6aec47460b6` and user-owned title. Never overwrite task
plan user edits: full versioned reads, exact unique edits or append, reconcile
conflicts, and inspect state after a lost append response. Unicode fragments
cannot replace full plans. Use discovered canonical Kandev schemas.

Design: no production/permanent test changes, install, test/lint/type/build/heavy
hooks, commit or push. Cheap catalog/spec/coverage/whitespace gates only. Local
heavy capacity is NONE here; CHILD87 owns the exclusive lease. End at this
package handoff; later ROOT implementation interrupt and separate heavy lease
are both required. No operator approval/model prompt and no inferred release.

Autopilot: continue reversible authorized work here. Only a critical unsafe or
uninferable blocker uses the direct-parent question tool, which immediately ends
the turn; never continue after that call. No child-to-ROOT interrupt. Optional
queued progress callbacks never gate completion or trigger full-queue retries.
Direct-readable checkpoints must suffice.

Later delivery preserves the caller's standing gates: meaningful permanent
RED/GREEN, exact serial checks; normal active pre-commit/commit-msg Conventional
Commit with no bypass/amend; normal push and ready PR with clean local/upstream
and authoritative remote head. Inspect caller-bound PR association for canonical
repository `16026b06-bd79-47c0-aed1-dc7ca95f63d9`, link only if absent, read back
all five automation fields false, complete/errors[]; no needless relink.

Return heavy explicitly only after all original native processes have actual
terminal joins, wait/reap receipts and fresh owned process/group absence. Exactly
one original `scripts/pr-await` all-terminal 90m/cadence60 with GNU91m/kill10;
retain and join it before replacement, no duplicate manual GitHub timer polling.
Freeze SHA except valid findings; no moving-main rebase, synthetic merged tests,
optional polish, weakened assertions/race/timeout/retry/policy gates. Six required
contexts SUCCESS and actual Backend/Frontend/E2E parent SUCCESS on exact head;
fresh complete/errors[], zero failed/pending/unresolved hidden actionable/human
findings. CodeRabbit App347564 must be authenticated/configured and substantively
cover every changed path at sourceCommitId=coveredCommitId=head, kind reviewed.
Inspect auto-full review gap/skip before at most one necessary full request/head;
no ACK, optional Claude or second review wait. Ground and disposition each thread
and grouped finding. Hosted retry belongs to ROOT only for exact failed leaf
after terminal parent/fresh head; never whole/passing/duplicate jobs.

If an actual backend-code fixup is required, checkpoint scope and run one full
changed Go lint against exact PR base: GOMAXPROCS2/GOMEMLIMIT1GiB, concurrency2,
allow serial runners, CLI5m/GNU6m/kill10. TS/docs changes do not trigger it.
Merge needs a separate ROOT serial normal expected-head squash grant, with no
admin/bypass/rebase. Prove actual MERGED, SHA/tree/owned blobs and remote inclusion,
join all originals, clean only owned resources and end. Preserve managed
worktree/dependencies/proof/foreign resources. ROOT owns independent verification,
archive-ABSENT/proof release and loop refills.

## Verification results

Design gates passed on 2026-10-08: catalog validation (365 decisions, 1472
specifications), all-specification lint, tracked whitespace, explicit whitespace
including both untracked package files, local document links, all AC/design
references and the exactly-one-work-order inventory. Standalone documentation
coverage for the four docs plus representative future two-hook source paths
reports `covered`, `ok: true`, `errors: []`. This is design traceability evidence,
not implementation evidence.

Receipts and per-gate raw logs:
`/tmp/kandev-child88-plan-draft-design-20261008/`. Original native start chunk
`f17997`, session `31003`, actual terminal chunk `997d45`, exit 0. Four gate
children were waited/reaped; wrapper PID/PGID/start identity and each child
identity are recorded in `gates.json`. Fresh original wrapper/child groups were
proven absent by terminal read-only check `5c5fde`, exit 0. Zero outstanding
handles; local-heavy lease remains NONE. No install, permanent test, production
edit, frontend lint/type/i18n, browser/build, hooks, commit/push/PR or merge ran.
Protected proof was inspected once without modification/reuse. All four changed
documents remain unstaged/uncommitted; later explicit ROOT implementation and
exclusive heavy release are required. Exact implementation checks are in the
one work order; their results remain pending.

ROOT's design review requested one precision correction before release: the
owning requirement's added scope prose now explicitly distinguishes implementation
accepted-payload matching (whole-draft preservation on text or attachment
mismatch) from ordinary submit's unchanged `004.5` text-only clearing on changed
attachments. Original acceptance criteria, paired design, work order and
production scope are unchanged. This remains a design continuation, not an
implementation release.

Affected cheap gates passed again on 2026-10-08: catalog validation (365 decisions,
1472 specifications), all-spec lint, whitespace and standalone reference coverage
(`covered`, `ok: true`, `errors: []`). Precision raw logs/receipts are retained
under `/tmp/kandev-child88-plan-draft-design-20261008/precision/`; original native
start `8a446b`, session `97763`, actual terminal `608888`, exit 0. Original gate
history is retained. Final precision checks verify original AC text, local
links/references, four-document hashes, unstaged state, wrapper/child wait/reap
and fresh owned-group absence. Zero outstanding handles; heavy and implementation
authority remain NONE. ROOT later releases the exact reviewed package after
CHILD87's explicit heavy return.

## Implementation phase checkpoint

ROOT explicitly released implementation in this same primary after both design
ends and accepted actual package review
`/tmp/kandev-root-child88-design-review-20261008.json` (`6af461`, exit 0).
Current authorization is AUTHORING ONLY while CHILD87 owns exclusive global-heavy.
Task 01 is in progress: inspect source/dependency presence and author independent
permanent first-party rendered tests/local real fixture. No install, product
tests/lint/type/build/heavy hooks, commit/push or production correction may run
until a subsequent explicit ROOT heavy grant after CHILD87's qualified RETURN.
Meaningful independent permanent RED must precede production edits after that
grant. `apps/node_modules` is currently absent; no installation has run.
All existing identity, question, review/delivery, proof protection and ownership
boundaries remain. This checkpoint supersedes the prior design-only hold only
for authorized test authoring; it does not infer local-heavy capacity.

## Risks

- Capturing the ref but later reading its current callback still reaches a
  successor. Capture the callback itself before the first await.
- Matching against the augmented prompt never clears unchanged drafts.
- A direct storage null write defeats a refusal by the shared owner guard.
- Fresh activation can retire the original editor; preserve its existing order
  and allow refusal rather than broadening scope to historical storage cleanup.
- Isolated hook mocks cannot prove actual live/persisted/editor restoration.

## Full implementation lease checkpoint

ROOT later granted this same primary exclusive global local-heavy after qualified
CHILD87 corrective RETURN (`/tmp/kandev-root-child87-corrective-return-qualified-20261008.json`,
original ROOT terminal `6334fa`, exit 0). The authoring-only checkpoint above is
historical. Task 01 remains in progress. Dependencies are absent; Corepack resolves
the existing apps packageManager pin to pnpm9.15.9 under Node24.21.0. Next is one
frozen install, independently authored permanent first-party RED before production,
then the exact serial scoped gates and normal publication. All original native
handles and process identities must be retained/joined; explicitly RETURN heavy
after publication/all joins before one original hosted observer. ROOT's separate
merge grant remains required. Proof and scope protections remain unchanged.

### Install qualification

The single frozen Corepack pnpm9.15.9 install exited 0 and original wrapper
1763806 wait-reaped child1763850. Both original groups are freshly absent.
Outer native start `7b6a11` returned 0 without a yielded session because `setsid`
forked; actual install terminal comes from the original wrapper receipt, without
claiming a native child join. ROOT independently reconciled this limitation,
unchanged manifests/lockfiles and original process absence (`871cc5`,
`/tmp/kandev-root-child88-install-reconciliation-20261008.json`). Do not replay.
Subsequent commands use attached wrappers with retained native sessions.

## Local implementation validation checkpoint

Both existing action hooks capture the initiating handle's accepted-payload
callback and unaugmented text/attachments before awaiting. Successful settlement
calls only that callback after unchanged side effects and fresh activation order.
No immediate glue, new API or shared composer change was needed. Ordinary
submission's text-only attachment-change policy stays unchanged.

Independent permanent RED03 (`740a92` / session13076 / terminal7e395a, exit1)
proved 15 causal failures and 6 passing controls with no unhandled errors.
The final exact eight-suite GREEN04 (`d0f863` / session13636 / terminalec6615,
exit0) passed all 96 tests after affected fixture/lint/type repairs. Scoped
ESLint02 and final helper lint, typecheck02, i18n check/ratchet, catalog validation
(365 decisions/1472 specs), all-spec lint, all-12-path reference coverage
(`covered`, `ok:true`, `errors:[]`) and whitespace passed. Actual receipts/logs
and original process identities remain in
`/tmp/kandev-child88-plan-draft-implementation-20261008/`.

Task 01 remains in progress at this pre-publication checkpoint solely for normal
publication, exact-head hosted review/CI and separate ROOT merge authorization.
Latest hosted/delivery status and immutable-head receipts are recorded in the
platform task plan rather than inferred from this committed local checkpoint.
