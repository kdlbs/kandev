---
created: 2026-10-24
status: draft
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003
system_design:
  - ../../specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md
legacy_specs: []
---

# Implementation Plan: Sentry Watch Duplicate and Instance Rebinding

## Overview

Let users copy a Sentry watcher and point the copy, or an existing watcher, at
another Sentry instance of the same workspace (one app, several regional
Sentry deployments). Order: the backend rebinding contract and stale-poll
guard first (independent), the frontend Duplicate flow with instance-scoped
field reconciliation in parallel, then the edit-mode rebinding UI that needs
both.

Decision record:
[ADR-2026-10-24-sentry-watch-instance-rebinding](../../decisions/2026-10-24-sentry-watch-instance-rebinding.md).

## Scope

### In scope

- Duplicate action in every watcher row (desktop table and phone list),
  between Reset and Delete, opening a prefilled create dialog. The copy keeps
  the source row's displayed Enabled state, including an unsaved toggle,
  without saving or discarding that source draft.
- Instance change in create, duplicate, and edit dialogs keeps the org and
  projects the new instance offers, clears the rest, and keeps every other
  field.
- PATCH `sentryInstanceId` with workspace validation. An effective change
  atomically clears import history, `last_polled_at`, and the last error, and
  keeps tasks.
- Instance/revision guards on reservation, task attachment, and release.
  Process-owned dispatch admission excludes rebind, Reset, and watch deletion.
  Reset owns its entire destructive operation; stale revision CAS and live
  operation conflicts return 409 before destructive effects.
- Edit-dialog warning, updated help text, public docs, and seven-locale copy.

### Out of scope

See the requirement's out-of-scope list: cross-workspace duplication,
server-side clone, per-instance history, unbinding, moving tasks, and other
integrations.

## Technical approach

See the system design. Key points:

- Task 01 adds PATCH rebinding and persisted `scope_revision` with fresh/
  legacy/replay migration coverage. Every update compares a supplied immutable
  opening precondition, or the server pre-read revision for older callers;
  rebind and Reset increment the stored revision. Instance and revision travel
  through all history mutations and polled events.
- A Sentry-service-owned admission coordinator tracks shared live dispatches
  versus exclusive rebind/Reset/deletion. Its dispatch handle spans the actual
  goroutine's coordinator call. Reset owns the full task-deletion loop without
  a DB lock across I/O. All store paths, including Reset and Delete, use
  parent-before-child locks. Abandoned SQL rows alone do not imply live work.
  Sentry's Reset wraps the handoff deleter to delete only each recorded task
  (`cascade=false`), preserving/reparenting non-recorded descendants. Shared
  watchreset and other integrations' cascade semantics remain unchanged.
- Task 02 adds `onDuplicate` to `WatchActions`, a `duplicating` snapshot in
  `SentryIssueWatchersSection`, a `duplicateOf` prop and `duplicate` opening
  mode in `SentryIssueWatchDialog`, lifted project loading with status, and
  the pure `reconcileInstanceScope` helper gating Save and scoped selectors.
  Duplicate initialization preserves empty stored prompts without changing
  the existing edit fallback.
- Task 03 enables the instance picker in edit mode, shows the warning
  `Alert`, sends `sentryInstanceId` only when it changed, and updates the
  help text, `FormState` comments, and both public immutability statements.
  It also adds Sentry-specific Reset scope/failure copy through optional
  shared-dialog props, leaving other integrations' defaults unchanged.

## ASCII UI preview

Structural requirements: action order (Pause/Play, Check now, Reset,
Duplicate, Delete), Duplicate being an icon button with tooltip and
accessible name, the dialog title stating "Duplicate", and the warning placed
directly under the instance picker in edit mode. Spacing, icon choice
(`IconCopy`), and exact wording are illustrative; copy is localized.

### UI-01: Watcher row actions (desktop table, Sentry settings)

Before:

```text
| Instance | Filter                 | Interval | Last polled | Status | Actions                       |
| US       | org:acme project:web   | 5 min    | 2m ago      | Active | [Pause] [Check] [Reset] [Del] |
```

After:

```text
| Instance | Filter                 | Interval | Last polled | Status | Actions                             |
| US       | org:acme project:web   | 5 min    | 2m ago      | Active | [Pause] [Check] [Reset] [Dup] [Del] |
```

Each bracket is an icon button with a tooltip; `[Dup]` is the new Duplicate
button.

### UI-02: Watcher row actions (phone list)

Same `WatchActions` component; at least 44 px targets on phone or coarse
pointers and 28 px on fine-pointer desktop. Reuse the shipped Sentry
`MobileWatchCard` and scrollable dialog, not a squeezed desktop table. The
card body opens edit; visible secondary buttons include Duplicate. The shared
dialog supports a deep form with one body scroll owner and reachable footer;
keep viewport containment and safe-area clearance. Share all draft and
reconciliation logic. The phone E2E must complete Create, not merely open it.

```text
+-----------------------------------------+
| US                                      |
| org:acme project:web env:production     |
| [Active] 5 min  2m ago                  |
|     [Pause] [Check] [Reset] [Dup] [Del] |
+-----------------------------------------+
```

### UI-03: Duplicate dialog, after switching instance US -> EU

Entry: Duplicate on the US watcher, then pick EU. EU offers org `acme` and
project `web` but not `api`.

```text
+------------------------------------------------------------+
| Duplicate Sentry watcher                                   |
| Workspace      [ Default workspace       v ] (locked)      |
| Sentry instance[ EU                      v ]               |
|   Which Sentry instance this watcher polls.                |
|------------------------------------------------------------|
| Organization   [ acme v ]        (kept: offered by EU)     |
| Projects       [ web x ]         (api dropped)             |
| Environment    [ production ]    (kept)                    |
| Levels / Status / Lookback / Query ...        (kept)       |
|------------------------------------------------------------|
| Workflow, step, repo, profiles, prompt, interval,          |
| in-flight cap, Enabled                         (kept)      |
|                                  [ Cancel ] [ Create ]     |
+------------------------------------------------------------+
Create is disabled while EU's org/project lists are loading.
```

### UI-04: Edit dialog with a changed instance

```text
+------------------------------------------------------------+
| Edit Sentry watcher                                        |
| Sentry instance[ US                      v ]  (was EU)     |
|   Which Sentry instance this watcher polls.                |
| +--------------------------------------------------------+ |
| | (!) Changing the instance clears this watcher's import | |
| |     history. Existing tasks are kept; matching issues  | |
| |     on the new instance are imported on the next poll. | |
| +--------------------------------------------------------+ |
| ... rest of form unchanged ...                             |
|                                  [ Cancel ] [ Update ]     |
+------------------------------------------------------------+
```

Phone: the dialog is the existing full-width scrollable dialog; the alert
wraps within the body. No separate phone composition.

### UI-05: Sentry Reset scope and failed preview

Shared desktop/phone confirmation; scope text remains visible in every state.

```text
Reset Sentry watcher
Delete 1 recorded task?                  (successful preview)
Could not load the current-history count. (failed preview)
Only tasks in this watcher's current import history are deleted,
including archived tasks. Tasks from before an instance change stay.
                                      [ Cancel ] [ Reset ]
```

Successful and failed preview lines are alternatives, not simultaneous copy.
Exact wording is illustrative and localized.

| Preview | Criteria |
| --- | --- |
| UI-01, UI-02 | `AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.1` |
| UI-03 | `AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.2`, `002.1`-`002.4` |
| UI-04 | `AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.1`, `003.2` |
| UI-05 | `AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.7` |

## Tests

- `001.1`: `sentry-issue-watch-table.test.tsx` asserts the action order
  (Reset, Duplicate, Delete) on desktop and phone, and that Duplicate calls
  `onDuplicate` without `onEdit`.
- `001.2`, `001.4`: dialog/form tests cover duplicate prefill, including an
  exact empty stored prompt and normal readiness correction, and the legacy
  instance requirement. The section test covers a displayed unsaved Enabled
  toggle copied without saving the source draft.
- `001.3`, `001.5`: dialog tests cover the create path and failed legacy
  filter create (multi-status and unsupported lookback), retaining the draft
  and source. E2E proves independent rows and identities.
- `002.1`-`002.5`: form/dialog tests cover project filtering by org, list
  errors, stale responses, sole-org handoff, and both selectors plus Save
  remaining disabled during deferred or failing verification. Attempts to
  toggle synthetic carried choices must not enable Save or keep unverified
  selections. Delete the obsolete eager-clearing test, not re-pin it.
- `003.3`, `003.5`, `003.6`: service tests cover rebind clearing, unchanged
  history, foreign/empty instance errors, and explicit legacy binding always
  clearing (current resolution equal, different, or unresolvable). Replace
  `TestService_UpdateIssueWatch_InstanceImmutable` with history-preserving
  omitted/equal cases in `..._KeepsHistoryWhenInstanceUnchanged`.
- `003.3`, `003.8`: migration/replay and store tests cover revision
  materialization, atomic rollback, and CAS conflicts including A->B->A
  while validation is paused. Preserve history imported after the first
  change; HTTP tests cover coded 409.
- `003.4`: history tests cover instance and revision matches/mismatches,
  including same-instance events from before Reset or A->B->A. Delayed old
  assign/release cannot touch a new scope's same-short-ID row.
- `003.9`-`003.11`: `TestSentryDispatch_OperationAdmission` pauses after
  Reserve: Reset, delete, and rebind reject with no effects; after Dispatch
  exits, retry succeeds. The real Sentry resetter/task-deleter test holds
  deletion while dispatch and rebind are refused, covers caller cancellation,
  and reverses the binding order. Failed-attach/restart coverage proves safe
  abandoned-row cleanup. Store lock-order cases include Reset and Delete.
- `TestStore_SentryWatch_LockOrder` uses shared file-backed SQLite and
  independent writer connections. `TestPostgresSentryWatch_LockOrder` uses an
  isolated schema and >=2 physical connections with barriers; distinct
  `pg_backend_pid()` values prove it is not a one-connection pool test.
  Task 01 requires a live DSN, explicit test names, and non-skipped execution.
- `003.1`, `003.2`, `003.6`, `003.8`: dialog tests cover editable binding,
  warning for bound and legacy edits, conditional payload, and failed PATCH
  retaining its draft. Delete the disabled-legacy-picker assertion.
- `003.12`: service/API tests reject a supplied opening revision that is stale
  before server pre-read, including ABA and Reset, with no side effects. The
  hook/section/dialog regression opens an old A form while an accepted rebind
  response is held, publishes B, then proves Save of the old opening conflicts
  and retains the draft rather than applying A's filter to B. Revision 0 and
  same-scope hydration remain valid; omitted expected revision preserves older
  callers' server-pre-read behavior.
- `003.7`: a real handoff-backed Sentry Reset test nests pre-rebind, unrelated/
  archived, and current-history children under a recorded root. Non-cascading
  per-ID deletion removes only current-history IDs in either enumeration order,
  preserving other tasks and valid ancestry. Desktop/phone edit E2E retain the
  previous-instance task; dialog tests cover truthful Reset scope on preview
  success, zero, loading, and failure. Other integrations remain unchanged.

## E2E tests

In `apps/web/e2e/tests/integrations/sentry-settings.spec.ts`, project
`chromium`, with two mock instances `US` and `EU`. Both offer org `acme`;
`US` offers projects `web` and `api`, and `EU` offers only `web`.

- `duplicates a watcher onto another instance` (`001.1`-`001.3`,
  `002.1`-`002.3`): create a US watcher (web and api, env `production`),
  click Duplicate, switch to EU, assert org `acme`, project `web` only, and
  env `production`, then Create. Assert two rows (US and EU) and an unchanged
  source via the API.
- `moves a watcher to another instance` (`003.1`-`003.4`): seed issue
  `WEB-1` on US, trigger, and assert one task. Seed a different issue with
  short ID `WEB-1` on EU, edit the watcher to EU, assert the warning, Update,
  trigger, and assert a second task whose metadata names EU. Open Reset,
  assert current-history scope, confirm, and verify EU's task is deleted
  while US's remains.
- Phone parity: `duplicates a watcher from the phone list` in
  `apps/web/e2e/tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts`
  (project `mobile-chrome`, Pixel 5) taps Duplicate in a phone card and
  asserts prefill, switches instance, completes Create, and verifies the copy
  and unchanged source. Measure Duplicate at least 44 px and check viewport
  containment. Reuse existing phone dialog geometry/lifetime coverage.
- Phone rebind parity: `moves a watcher from the phone list` in that same
  `mobile-sentry-watch-dialog-ownership.spec.ts`, project `mobile-chrome`.
  Tap edit, change US->EU, reach the warning and Update footer with contained
  internal scrolling, verify changed-instance PATCH and the EU collision
  import, then Reset and verify EU deleted/US retained. Existing unchanged-edit
  ownership tests do not cover this path.

## Work orders

- [ ] [Task 01: Backend Sentry watch instance rebinding](task-01-backend-instance-rebinding.md) (wave 1)
- [ ] [Task 02: Duplicate action and instance-scoped field reconciliation](task-02-duplicate-and-scope-reconciliation.md) (wave 1)
- [ ] [Task 03: Edit-mode instance change UI](task-03-edit-instance-change-ui.md) (wave 2, depends on 01 and 02)

## Verification

Each work order lists its targeted commands. After all orders:

```bash
make fmt
make typecheck test lint
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts --grep "duplicates a watcher from the phone list|moves a watcher from the phone list")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Verification results

Implementation checks are pending. No production or permanent test changes
are authorized by this design-review loop.

Design-only validation after round 1 corrections:

- `python3 scripts/list-docs.py validate`: passed (404 decisions, 1730 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check HEAD -- docs/specs docs/decisions docs/plans/sentry-watch-duplicate-and-rebind`: passed.
- Local `.github/scripts/pr-docs.cjs` preflight: exempt for docs-only changes.
  Explicit frontmatter/link validation passed for all 3 work orders and their
  25 acceptance references.

Design-only validation after round 2 corrections:

- `python3 scripts/list-docs.py validate`: passed (404 decisions, 1730 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check HEAD` over the requirement, design, ADR, and plan package:
  passed.
- Frontmatter/link and coverage validation: all 3 work orders, 29 acceptance
  references, and every one of the 21 unique acceptance criteria covered.
  PR-documentation preflight remains exempt for these docs-only changes.
- No product tests/builds/browser checks run: implementation remains pending.
  Round 3 was not launched because current runtime evidence proves the
  requested Opus model is unavailable and the selected profile falls back.

## Adversarial review progress

- Round 1: 13 findings. Addressed all in the package: history mutation guards,
  always-clear legacy binding and warning, obsolete tests, exact test/runner
  commands, truthful Reset copy, missed immutability text, explicit agent
  settings exclusion, instance-only browse keys, sole-org handoff, displayed
  Enabled semantics, legacy-create failure, and original-binding conflicts.
- Round 1's reservation-only fence is superseded: SQL empty rows cannot prove
  live ownership, and Reset could erase them. The corrected design uses
  process-owned operation admission plus persisted scope revision.
- Round 2: child `18c0ca80-5411-495a-a84a-33806e6a8e32` returned 8 findings
  plus one scoped-guidance inventory note. Addressed in the package:
  - F1/F2: enforced dispatch/Reset/rebind/delete admission, full Reset ownership,
    revision-scoped finalization, and safe abandoned-row recovery.
  - F3: Reset and Delete join the parent-before-child transaction cutover.
  - F4: scoped selectors cannot toggle synthetic carried options while pending.
  - F5: persisted monotonic revision CAS catches A->B->A.
  - F6: lossless empty-prompt duplicate initialization, edit fallback unchanged.
  - F7: explicit migration/concurrency tests, fail-fast DSN prerequisite,
    independent scoped PostgreSQL connections, file-backed SQLite.
  - F8: named mobile changed-instance edit/Reset E2E and exact runner command.
  - Guidance note: Task 01 updates the integration scoped guidance when shipped.
- Routing evidence: round 2 requested Opus profile
  `3365c84c-1a44-491f-a8bb-f0df46b06479` but backend logs for execution
  `0427101f-31e2-4116-be2c-ee9b2b42f30c` explicitly record unavailable
  `anthropic/claude-opus-5-5`, fallback application to
  `openai-codex/gpt-6.1-sol`, and that confirmed current session model.
  Source: diagnostic bundle `35787f32e437a1a101fe3cdba906d5a2`,
  `backend/backend-logs.log`, lines 31109-31122. Round 2 is static Sol evidence,
  not a runtime-confirmed Opus review. Current agent catalog also reports the
  Opus model unsupported.
- Round 3 is blocked on Opus availability. Do not start another known-fallback
  child or change the user's global profile to conceal the mismatch.
  Consecutive clean rounds: 0. User stop rule remains 50 total rounds or 3
  consecutive clean rounds. Resume with a supported Opus route, verify runtime
  model metadata, then create the next read-only child; it messages findings
  and archives itself. Do not poll/wait synchronously for reviews.
- Continuation probe: fresh `omp models find opus --json` found the catalog
  route `litellm-fi/claude-opus-5`, but catalog presence is not availability.
  A fresh OMP 18.12.6 ACP handshake (protocol 1) advertised eight model options,
  none Opus; its default was `openai-codex/gpt-5.6-terra`.
  Direct invocation with explicit `--model litellm-fi/claude-opus-5`,
  `--no-tools --no-session --print --mode json` returned an assistant error
  with HTTP 403, `litellm.PermissionDeniedError: Model is blocked`, and zero
  usage. No review was performed and no third child was created.
  ACP frames are in `/tmp/sentry-opus-acp-dJ5MtL/frames.jsonl` (ephemeral
  diagnostic evidence). The Go probe helper could not build because the
  home cache was full and an isolated build hit disk quota; a direct stdio
  handshake supplied the runtime evidence instead. Its isolated cache was
  removed. Continuing requires an authorized Opus route or explicit user
  permission to change the review model; "continue" did not authorize a
  silent model substitution.
- User subsequently selected "Use Sol 6.1", explicitly authorizing remaining
  reviews on that model. This supersedes the Opus routing blocker above.
  Round 3 child `6135e323-9513-4f8c-a80c-5ddca96f9efd` was created with
  auto-start requested, Sol profile `af8936d3-6107-49fc-96a7-f4a488773939`,
  and the inherited workspace. It reviews read-only, messages findings, and
  archives itself. Await its message, not synchronous polling. Clean streak
  remains 0; the 50-round / 3-consecutive-clean stop rule is unchanged.
- Round 3 returned 3 P2 findings; not clean. Corrected: read-only response
  revisions guard hook/list publication and subsequent Edit/Duplicate
  baselines; sole-instance defaults enter the same reconciliation transition;
  DELETE uses the coded conflict mapper with named DELETE/Reset HTTP tests.
  Work orders include the owning hook/new regression suite and exact commands.
  Reviewer supplied backend model-policy/current-model evidence confirming
  Sol 6.1 for its session. Clean streak remains 0.
- Round 3 corrections passed `list-docs.py validate`, full specification lint,
  and the package whitespace gate. Round 4 child
  `8f5d549d-cb97-4b19-a885-c571b08c88e2` created with auto-start requested
  on the authorized Sol 6.1 profile; awaiting its findings by message.
- Round 4 returned 2 findings (P1/P2), not clean. Corrected:
  - Reset now uses a Sentry-only non-cascading task-deleter adapter with a real
    mixed-provenance/archived hierarchy regression, preserving the promised
    current-history boundary and documenting child detachment.
  - Every edit opening carries immutable `expectedScopeRevision`, checked
    before patch application and at store CAS, so a stale open form conflicts
    even before server pre-read. Older callers may omit it; response revision
    guards remain necessary but are not an opening precondition.
  Work orders/requirements/design/ADR agree; clean streak remains 0.
- Round 4 corrections passed catalog validation (404 decisions/1730 specs),
  full spec lint and whitespace. Traceability passed for 3 work orders,
  31 acceptance references, and all 22 unique criteria.
  Round 5 child `f7dd7909-f1eb-495a-bebc-bc4ff1e8236d` created with
  auto-start requested on the authorized Sol 6.1 profile; awaiting its findings.
- Round 5: ZERO findings after source-backed design review. Consecutive clean
  rounds: 1 of 3 required. No implementation or product verification implied.
  Round 6 child `630833d6-38ef-44f0-a280-09c88198299f` created with auto-start
  requested on the authorized Sol 6.1 profile; awaiting findings by message.
- Round 6 returned one P2 dependency finding; clean streak resets to 0.
  The non-cascade handoff normalization currently rejects archived inheriting
  children. Task 01 now owns the archived-aware normalization cutover while
  retaining observed parent/mode/orphan CAS and compensation, plus focused
  handoff and real Sentry Reset regressions with archived recorded/non-recorded
  inheriting children in both deletion orders. Other integrations' cascade
  behavior and active-only orphan stamping guards stay unchanged.
- Round 6 corrections passed catalog validation (404 decisions/1730 specs),
  full spec lint and whitespace. Round 7 child
  `fd2e10c6-85ea-49fb-a8a6-7c3536005b66` created with auto-start requested on
  the authorized Sol 6.1 profile; awaiting findings by message.
- Round 7 returned one P2 child-set race; not clean, streak remains 0.
  Task 01 now owns atomic observed-child normalization/detachment under
  repository hierarchy admission, exact child-set/workspace guards, rollback
  on drift, and retention of the final root-delete check and conditional
  compensation. Added real repository/handoff-backed Sentry Reset arrival
  regressions plus focused SQLite/PostgreSQL repository commands and ownership.
- Round 7 corrections passed catalog validation (404 decisions/1730 specs),
  full spec lint, whitespace and traceability (3 work orders/31 references/
  all 22 criteria). Round 8 child `74176d89-c6f8-442c-9359-689b2cac01a6`
  created with auto-start requested on the authorized Sol 6.1 profile;
  awaiting findings by message.
- Round 8: ZERO actionable findings in source-backed static design review.
  Consecutive clean rounds: 1 of 3 required; not product verification.
  Round 9 child `bfb292c7-42e9-4304-8cf2-9cdc814fc010` created with auto-start
  requested on the authorized Sol 6.1 profile; awaiting findings by message.
- Round 9: ZERO actionable findings in source-backed static review.
  Consecutive clean rounds: 2 of 3 required.
  Round 10 child `21ef780a-2702-4e90-9f3f-bb36a09f8416` created with
  auto-start requested on the authorized Sol 6.1 profile. If clean, stop the
  review loop at three consecutive clean rounds; otherwise address findings
  and reset the streak. Await its message, not synchronous polling.
- Round 10: ZERO actionable correctness/completeness findings. Rounds 8, 9,
  and 10 are three consecutive clean source-backed static reviews.
  User stop condition satisfied after 10 total rounds; review loop complete.
  No round 11 will be created. Implementation remains pending and requires
  explicit user authorization; the clean review streak is not product
  verification. All work orders remain pending and design artifacts remain
  uncommitted for the user-controlled implementation handoff.

## Risks

- Changing all three history-mutation signatures touches store callers and the
  orchestrator fake; a missed caller must be caught by package compilation.
- Scope revision must survive migration/replay; an instance-only comparison
  does not catch A->B->A. Reset and Delete must follow the same parent-first
  SQL lock order as all history mutations.
- A process-owned handle must span actual Dispatch and all Reset deletion I/O,
  not merely bus delivery or the final store transaction. This uses the existing
  single-backend-replica contract; PostgreSQL alone does not add replica safety.
- Empty abandoned reservations are not live-operation evidence. Never infer
  abandonment from elapsed time while a Dispatch handle is active.
- A stale poll's deferred `last_polled_at` and error stamps can land after a
  rebind (bounded by one interval; see the design). It cannot create tasks.
- Switching back to a previous instance re-imports its current matches
  (accepted in the ADR).
- Reconciliation depends on loading status from two hooks. Late responses for
  an earlier instance must be discarded by request key, or a stale list could
  wrongly keep or drop selections.
- Changing the English `sentryInstanceHelp` value requires retranslating six
  locales; `i18n:check` rejects values identical to English.
