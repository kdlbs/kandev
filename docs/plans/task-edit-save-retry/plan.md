---
created: 2026-10-08
status: implemented
requirements:
  - REQ-TASKS-EDIT-SAVE-RETRY-001
system_design:
  - ../../specs/tasks/system-design/task-edit-save-retry.md
legacy_specs: []
---

# Implementation plan: Task edit save retry

## Overview

Keep a rejected current task edit open for correction/retry, without changing
confirmed-stage semantics. ONE sequential work order independently authors the
permanent regression, corrects the two edit catch boundaries, and validates the
affected frontend and documentation contracts.

This package began as DESIGN ONLY. Task `cc9b719f-0081-4585-b970-b761885dec6f`, session
`56c8ff66-fd98-4bc1-ab42-f48ab0bad1c8`, ROOT slot86. The versioned platform plan
preserves the `<kandev-system>` marker and binding delivery gates. Same primary,
no delegates/tasks/sessions/model switches. WAIT for a later explicit ROOT
reviewed-package implementation INTERRUPT; creating this package does not admit
implementation. Files stayed unstaged/uncommitted at that checkpoint. ROOT later
reviewed all four artifacts and released implementation to this same primary
with exclusive local-heavy86. Review receipt:
`/tmp/kandev-root-child86-design-review-20261008.json`. No merge grant exists.

## Scope

- Ordinary failed current task-field save/retry, both edit variants, editable
  raw draft values, failure feedback, busy cleanup, confirmed data, and success.
- Preserve branch-policy, runner, dependency, repository, partial-launch, and
  existing admission behavior through targeted compatibility evidence.
- Public-doc impact audit and a small recovery paragraph at implementation.

Navigation/unmount persistence, global drafts, backend/API/transaction changes,
fresh-branch policy expansion, other editors, dependency additions, broad suites,
and optional polish are excluded.

## Evidence and assumption check

Confirmed intent: retain failed edits in the current editor and permit retry;
normal success closes. Source verified the two catches and open-reset/remount
chain at base `b232b2931a330864975d90d867d320a3239db8e4`.

Accepted parent proof: `/tmp/kandev-root-task-edit-retry-discovery-20261008/qualified-proof.json`;
protected candidate hash
`6a72ff26e391467812e2afb4c8a16e26b92d6334b770a16454c88b655870c754`.
READ ONLY: no replay, copy, import, or modification. Original native
`71844/dd528e/ACTUALJOINd228ac`, exit 1: one causal title-loss plus two positive
controls through real hooks/API/providers and a rendered edit/reopen adapter,
not the full dialog. Instructions reset was observed, but the title assertion
failed first; no independent body-loss PASS is claimed. Earlier matcher/dependency
fixture failures `94382`/`76307` are noncausal, never product RED.

Owner: Tasks owns acknowledged task metadata and save stages; UI only presents
them. Existing sidebar/dependency/runner contracts do not define ordinary field
failure retention. Add one vertical pair to that existing system; no duplicate
UI spec, legacy migration, system README inventory, or architectural ADR change.
No material product question remains. Full real-consumer fixture feasibility is
an implementation risk at design handoff; the independent full-editor RED/GREEN
now qualifies that boundary.

## Technical approach

Follow [the design](../../specs/tasks/system-design/task-edit-save-retry.md):
make both caught edit failures keep the current dialog open, retain stale-policy
refresh and typed message/partial-stage handling, and remove only dead
classification helpers. Reset/open-cycle behavior remains untouched.

## ASCII UI preview

UI-01: Existing task editor after an ordinary failed save. Entry: card/task
menu/sidebar Edit, including the phone task-switcher action. Before: source and
parent rendered evidence show closure; reopening loads confirmed old fields.
After (shared field/action order, illustrative localized labels):

```text
[existing Edit task surface stays open]
 Title:        [  Retry title v1  ]
 Instructions: [  Retry instructions v1             ]
               [Second line                        ]
 [existing failure toast: Failed to update task]
 [Cancel]                              [Update]
```

Desktop retains its inset dialog; phone retains its existing full-height editing
surface and form scroll region. UI-01 specifies retention and usable retry, not
new spacing, copy, controls, focus policy, or geometry. Started-task instructions
remain locked. AC-001.1/.2/.4/.6 map to the rendered regression below. The
`/mobile-parity` pure-state exception applies; no browser check is claimed.

## Tests

Primary owner: new `task-create-dialog-save-retry.test.tsx`, rendering the actual
first-party editor consumer and transport. Exact cases and literal oracles are
in [Task 01](task-01-preserve-failed-edit.md). RED independently demonstrates
ordinary title and instructions failures, with current success and stale-policy
controls collected in that same single RED run. GREEN runs the same regression
once after correction and the affected compatibility files once. No separate
passing-control replay or broad suite.

| AC suffix | Evidence |
| --- | --- |
| .1, .3 | Separate title/instructions raw-draft cases, real PATCH failure, confirmed consumer/store values and no success publication |
| .2 | Failed busy state settles; current edited retry payload succeeds and closes |
| .4 | Pending ordinary form submit, started update-only, pending split update-only |
| .5 | Stale-policy control plus existing runner/repository/dependency/partial-launch submit cases |
| .6 | Shared real editor consumer/reset lifecycle, explicit cancel control, source audit of phone consumer |

## E2E tests

No Playwright additions or local browser run. This is a state-only correction in
an existing shared editor. The integration test supplies end-to-end evidence
across real consumer, hooks/reset, rendered fields, production API client,
transport response, feedback, and close lifecycle. It is not a browser or server
integration claim. Existing mobile tests are not claimed to cover this failure.
If mounting requires a different boundary, ROOT must disposition it first.

## Public docs and i18n

Audit found `docs/public/tasks-and-workflows.md` owns task operation and recovery.
At implementation add one short how-to paragraph to Troubleshooting: a failed
edit stays open with unsaved values; correct/retry or explicitly cancel. Describe
only acknowledged outcomes, preserving existing partial-save warnings. No new
page, navigation, screenshot, terminology, or diagram is needed. This turn
changes internal design docs only. No UI copy or locale changes are planned;
existing translated toast and controls remain. Run the i18n gates named below.

## Work orders

- [x] [Task 01: Preserve failed task edits](task-01-preserve-failed-edit.md),
  done locally; sequential; no dependencies. No parallel/delegation wave.

## Verification and delivery gates

Task 01 owns exact commands and caps. The design turn permitted only catalog/spec-lint,
documentation reference preflight, diff/status, and artifact inspection. No
product tests, dependency install, lint/typecheck/build/browser, staging,
commit/push/PR. Later heavy commands require ROOT's single global lease and
native handle/process-group receipts. All handles must actually terminate and
be joined before heavy RETURN; resource/timeout/transport/unknown/outscope
failures checkpoint ROOT before alternatives or retries.

Later publication uses normal hooks/new Conventional Commit, caller-bound
canonical repository `16026b06-bd79-47c0-aed1-dc7ca95f63d9` association with all
five automations false, frozen published head, one joined original `pr-await`,
exact-head required contexts/parent workflows and substantive CodeRabbit full
coverage. Hosted retries need exact ROOT grants; MERGE needs a separate serial
ROOT grant. The platform plan contains the full binding constraints and is
authoritative for crash/continuation receipts.

## Verification results

Design receipts, 2026-10-08 (original native tools completed synchronously;
no running session handles):

- `python3 scripts/list-docs.py validate`: native chunk `4b6e6c`, exit 0;
  validated 364 decisions and 1470 specifications.
- `python3 scripts/lint-spec-files.test.py`: native chunk `ab5b4d`, exit 0;
  36 script tests passed. These are lightweight design-validator tests.
- `python3 scripts/lint-spec-files.py --all`: native chunk `cdf95e`, exit 0;
  all specification files passed.
- Read-only Node `validateCoverage` preflight: native chunk `9fc4ea`, exit 127;
  `node` was not found in this session PATH. The checker did not execute; no
  reference-coverage PASS is claimed. ROOT checkpoint persisted in the platform
  plan; no install, retry, alternative runtime or command was attempted.
- Initial `git diff --check`, status, cached-list and size inspection: native
  chunk `6b833d`, exit 0; four new artifact files, no staged changes. The diff
  gate alone does not inspect untracked content; final content inspection is
  recorded separately in the platform plan.

At the design handoff, product checks and delivery had not run. Child missing-Node
`9fc4ea` remains a historical failed check. ROOT then qualified the references
with read-only native `ac03b4`, exit 0 (four actual documents exempt, projected
production path covered, both `errors: []`), in
`/tmp/kandev-root-child86-reference-preflight-20261008.json`. ROOT authorized the
existing Node 24.21.0 PATH under non-login Bash; no runtime was installed and
the child's design-only preflight was not replayed as a synthetic pass.

Historical resource checkpoint, 2026-10-08:

- Independent full production editor RED: native `901b8a`, session `64396`,
  actual terminal `ede390`, exit 1. Seven separate draft-retention failures;
  stale-policy, successful save, and explicit cancel/reopen controls passed.
  Earlier fixture failures remain noncausal and are recorded in the platform plan.
- Minimal two-catch correction; reset, save ordering and error messages unchanged.
- Four exact affected files GREEN: `d41622` / session `90806` / actual terminal
  `7969eb`, exit 0, 62 tests. Following scoped fixture lint refactoring, only the
  changed new regression reran: `7cfbeb` / session `30840` / `356b5b`, exit 0,
  all 10 cases. The 52 unchanged compatibility controls were not replayed.
- Affected ESLint: `3d68f1` / session `1953` / `c469c2`, exit 0. One later
  literal-initializer fixture correction still requires affected lint coverage.
- Typecheck: `a79481` / session `82607` / actual terminal `7834b5`, exit 134:
  Node exceeded the reviewed 2048 MiB heap cap. This is a resource failure,
  not a typecheck pass or diagnosed TypeScript failure. No retry/cap increase.
- I18n, final documentation/reference gates, hooks, publication and hosted wait
  remain pending. Requirement/design lifecycle stays draft pending conformity.
- Every original heavy handle was joined and child wait/reap recorded; fresh
  wrapper and child groups are absent (native `6413cf`). Resource checkpoint
  returns heavy86; no remaining command starts without ROOT disposition/regrant.

ROOT later qualified the OOM in
`/tmp/kandev-root-child86-typecheck-oom-qualified-20261008.json`, confirmed sibling
84 is hosted-only, and regranted exclusive heavy86. Exactly one recovery retains
the same typecheck command/GNU 6m/kill-after 10s, changing only the heap to 4096
MiB. No runtime/install/cache/config/TypeScript-flag change is authorized. Original
exit 134 remains historical failure; the recovery result is recorded separately.

Final implementation verification, 2026-10-08:

- The resource recovery completed compilation and reported only new-fixture
  type errors. Those state-shape/timer/selector errors were corrected without
  changing production behavior, assertions or compiler flags. ROOT's explicit
  conditional own-code diagnostic permission admitted affected verification.
- Final changed new-file run: `93dd15` / session `65348` / actual terminal
  `ed9907`, exit 0, 10/10. The unchanged 52 compatibility cases retain the
  original 62-test GREEN receipt above; no passing-control replay.
- Final typecheck: `bc7748` / session `2466` / `7ec77b`, exit 0, same command
  and timeout with the documented 4096 MiB exception. Earlier resource exit
  134 and compiler-diagnostic exits 2 remain historical failures.
- Final affected ESLint: `395625` / session `32808` / `e9aecc`, exit 0.
- `i18n:check`: `9fb119` / session `69741` / `e2bdb1`, exit 0. Existing
  orphan-key notices are advisory; no copy/locales changed.
- `i18n:ratchet`: `260421` / session `98500` / `9387ff`, exit 0.
- Public-doc validator tests: `df8ce5`, exit 0, 62 cases; validator `24b430`,
  exit 0, 47 published pages.
- Actual seven-path reference preflight: `1ba7c0`, exit 0, `covered`, `ok: true`,
  `errors: []`, one accepted requirement/design pair and one coherent work order.
  This reads worktree contents; it does not claim hosted/published-head evidence.

Specification catalog validation `97e531` and specification lint `4c167c`
passed after the active/current lifecycle updates. The unchanged 36 script
tests retain their design receipt; no passing-validator replay was needed.
Local implementation conforms to all six ACs through the real state integration,
first-party mobile wiring audit and existing partial-stage controls. Normal
hooks, publication, hosted exact-head evidence and ROOT's
separate merge grant remain delivery gates in the platform plan.

Detailed native receipts and full raw output live in the task-owned
`/tmp/kandev-child86-receipts-20261008/`; protected parent proof remains untouched.
No browser/build/backend checks or coverage are claimed. No staged changes,
commit, push, PR or merge exists at this checkpoint.

## Risks

- Real dialog fixture must satisfy read-side hydration, dependency readiness,
  editor and provider contexts without replacing the causal production chain.
- Existing partial commits intentionally reconcile acknowledged fields; a broad
  draft guarantee must not erase that distinction.
- Transport failure cannot establish whether a remote write committed. Retry
  remains subject to existing server/admission behavior.
