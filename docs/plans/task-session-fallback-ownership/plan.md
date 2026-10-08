---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
legacy_specs: []
---

# Implementation Plan: Task session fallback ownership

## Overview

Correct task identity ownership of fetched fallback and request status in the
existing mounted resolver. One sequential Task 01 supplies real-hook and
reachable consumer coverage, the smallest correction, and focused validation.
ROOT reviewed the design package and released implementation in this same
primary session/profile. Local implementation is verified; hosted delivery
remains subject to the gates recorded below and ROOT's separate merge lease.

## Owning contracts and settled choices

Extend [task navigation responsiveness](../../specs/ui/requirements/task-navigation-responsiveness.md),
which already owns reusable client navigation isolation (`.5`, `.7`). Task
membership/eligibility remains task-owned. New `.8`-`.11` specify the missing
mounted fallback outcomes; this is not an incident-named duplicate capability.
The paired design covers the local resolver separately from shared read caches.
No material intent question remains: ROOT explicitly supplied scope, transport,
priority, proof, resource, delivery, and handoff constraints. Existing historical
navigation/refresh plans retain their statuses, results, and broad test history;
this follow-up neither reopens nor replays those packages.

## Scope

### In scope

- Task-keyed local fallback/request state in `hooks/use-task-session.ts`.
- Faithful real-hook/store tests and reachable `KanbanWithPreview` integration.
- Internal contract extensions and one work order with exact validation.

### Out of scope

- Store/global cache, API/backend, membership or canonical-primary policy.
- Same-task cache freshness, retry policy, generic ownership framework.
- Navigation/layout/copy/schema changes or remounting consumers.
- Delegates, tasks, sessions/tabs, model changes, browser/build/E2E/full suites.

## Accepted evidence and current source

ROOT owns `/tmp/kandev-task-session-selection-repro.test.tsx`, SHA256
`af7baaca64ea8635a6ecaa064840fe8fc504dedfc1af6a2f117217a188bb7e55`, and
`/tmp/kandev-root-task-session-selection-proof-receipt.json`. Read-only audit
confirmed the checksum, receipt, corrected log and exit receipt. Do not replay,
alter, remove, copy into a temporary executable suite, or archive this proof.
ROOT preserves it until independent verification and archive.

Corrected run 4857 was actually joined by ROOT: exit 1, 2.84 s, two semantic
wrong-prior-task fallback failures, two current/store-priority controls passed,
no setup/teardown errors. Initial 62436 was also joined, exit 1, but had extra
fixture-only beforeEach-returned-mock teardown errors. Its honest retained
receipt is not clean causal evidence. The corrected proof is authoritative;
ROOT checksum-cleaned temporary source and left production untouched.

Proof baseline: `0eb74e57332900b7bd5ca756c541d16170000492`.
Child audit HEAD: `8776877048ea029b8f861b4d658be719dfbadff2`.
Current hook blob `83f7148865af8d5f689a2ff45fad2abb09c7e534` and consumer blob
`958dd3bef1c8140a7213c82f07804d90027d4934` match supplied proof identities.

Cause: unkeyed `fetchedSessionId` survives settled Alpha fallback, then leaks on
uncached Beta while its request is pending, including Alpha/null/Beta. Existing
request cleanup guards late settlements but does not hide a previously settled
fallback on the new-task render. Store primary-or-first priority is correct.

## Technical approach and consumer audit

Use one task-keyed local snapshot and render-time ownership projection, retaining
per-effect active cleanup. See the design's [mounted fallback section](../../specs/ui/system-design/task-navigation-responsiveness.md#mounted-task-session-fallback-ownership).
Do not reset only in an effect, which misses the first new-task render.

The only production caller is `KanbanWithPreview`. Its workflow focus, user
selection, and task primary metadata override this fallback. When all are absent,
`useSyncSelectedTaskActivity` and `useUrlSync` can propagate Alpha's ID as Beta's
active selection/URL; current store actions do not validate membership.
`PreviewSessionTabs` separately validates preferred membership through
`pickActiveSessionId` and suppresses chat until its list loads. User effects
are therefore limited to this reachable projection, not a claim that every
preview shows the wrong conversation or that backend execution is admitted.

## Tests

New `hooks/use-task-session.test.tsx`, describe `task session fallback ownership`:

| AC suffix | Behavioral cases |
| --- | --- |
| `.8` | Settled Alpha to pending uncached Beta; Alpha/null/Beta; old success and rejection leave Beta result/loading intact |
| `.9` | Current marked primary wins over fallback even when not first; no-primary first-store wins; store takeover during read; first-list ID wins without new primary inference |
| `.10` | Current pending/loading then success, empty, rejection, no client; null selection has no session/loading; unmount cleanup |
| `.11` | Two mounted instances using real store retain independent results and loading; switching/settling one leaves the other intact |

New `components/kanban-with-preview.session-ownership.test.tsx`, describe
`Kanban preview task session fallback ownership`: real consumer/providers/hooks,
only WS transport mocked plus the explicitly reviewed marker-scoped viewport
fixture recorded below. Keep selected tasks without primary metadata, hold
Beta list requests, and assert actual URL and real active-store projection do
not adopt Alpha after switching or close/reopen. Resolve current Beta and assert
its ID is adopted. A primary-metadata control proves the mitigating precedence.
Actual rendered component composition is retained; unrelated WS calls receive
small protocol-valid responses. Do not replace integration with a synthetic
copy of focus logic or mock the resolver/store. If mounting reveals an unknown
runtime/transport dependency, checkpoint ROOT rather than widening scope.

## Mobile and browser evidence

Pure state/data correction after actual caller audit qualifies for mobile-parity's
unit/component exception. The existing phone direct-navigation branch skips the
preview pane but leaves shared hooks mounted. No rendered structure, interaction,
scrolling, breakpoint, or copy changes; no ASCII UI preview is applicable.
No browser/build/E2E/full-suite/backend commands are authorized for this fix.

## Work orders

- [ ] [Task 01: Scope fallback and request state to the selected task](task-01-task-session-ownership.md)

One work order, no dependencies, sequential only. ROOT retains one GLOBAL
LOCAL-HEAVY lease; no install, test, lint, typecheck, or heavy hook before release.

## Documentation impact

Internal docs only. The docs-maintainer audit found no changed command, setting,
public API, workflow instruction, label, navigation, or screenshot contract in
`docs/public/tasks-and-workflows.md`, root README, or screenshot catalog.
Existing user-facing task/session documentation describes the intended behavior;
no public documentation change is necessary for this internal ownership repair.

## Verification results

Design-only checks completed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: exit 0, 351 decisions and 1,368 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files pass.
- Catalog discovers the owning requirement/design pair. `git diff --check` passes.
- `.github/scripts/pr-docs.cjs::validateCoverage`: actual four documentation paths
  are exempt; the separately labelled prospective hook path is covered by Task 01,
  with complete requirement/AC/design/plan references and zero errors, exit 0.
- Plain `node` initially failed to start (exit 127, PATH missing). Read-only runtime
  discovery found existing `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`;
  that binary completed reference coverage without install or environment mutation.
- All design command handles returned terminal exits; no live owned handles.
  Production hook/consumer blobs match proof identities. Four artifacts remain
  unstaged/uncommitted. No production/permanent tests/install/heavy check executed.

Implementation checks remain pending. Public-doc validation, package eslint,
typecheck, localization, and permanent regression runs are later Task 01 checks.
Before later package commands, make the existing Node/toolchain discoverable in
that command environment; do not install Node or change shared configuration.
No public documentation, navigation, UI markup, package or backend changes.

## Risks

- Effect-only clearing misses the transient first new-task render.
- Old failure/finally settlement can clear current loading if ownership cleanup
  is weakened; test while Beta remains pending.
- Higher-priority selections and tab membership checks can conceal the hook
  defect in a consumer fixture; deliberately omit metadata for the causal case.
- Unknown consumer fixture dependencies or resource failures require ROOT's
  bounded direction, never automatic installation/recovery or wider testing.


## Implementation checkpoint: real consumer fixture

At the initial implementation checkpoint, Task 01 was blocked at the real
consumer fixture boundary after ROOT's later release. The permanent hook suite proves two causal REDs
(first Beta render after settled Alpha, and after null close/different reopen)
with 12 compatibility controls passing. Production remains untouched.

The actual consumer mounts through real StateProvider, TooltipProvider,
ToastProvider and CommandRegistryProvider, using real workflow/snapshot/store
selection and only a WS mock. Its real virtualized task list receives a
zero-height Happy DOM viewport and mounts no Beta card, so its three cases fail
before the switching/metadata assertions. A geometry shim or virtualizer mock
would exceed the current transport-only fixture constraints; no such change,
browser run, new dependency or production workaround was attempted.

Task 01 records all actual commands, terminal handles and logs. One permitted
frozen install succeeded. The approved typecheck generated normal ignored inputs
but initially failed on two corrected fixture type omissions; no passing
final typecheck or consumer proof claimed. All local handles actually joined;
none live. LOCAL-HEAVY returned to ROOT. Await bounded fixture direction and a
renewed exclusive lease in this same primary session/profile. No publication
or merge has occurred; all delivery gates remain pending.

## ROOT bounded viewport-fixture release

ROOT renewed the exclusive GLOBAL LOCAL-HEAVY lease after reviewing the joined
checkpoint and real virtualizer source. The prior blocked results above remain
historical. Task 01 resumes in the same primary session/profile.

One explicit test-environment exception to transport-only mocking supplies
`offsetHeight=600` and `offsetWidth=320` only on real elements matching the
existing `data-testid="kanban-column-scroll"` marker. Preserve original getters
for every other element, install before mount, and restore after every test.
The real virtualizer, cards, provider/store, consumer, selection/actions, hook
and URL updates remain active. No other geometry/framework mocks are authorized.

First run only the affected consumer suite with the exact existing project,
anchored name, worker and memory limits. Require two causal switching REDs and
the current-primary control PASS before changing production. Retain the prior
hook RED receipt without replay. If the scoped viewport still fails to expose
real cards or reveals a distinct runtime boundary, join and checkpoint ROOT
without expanding the fixture. Then apply the smallest hook correction, run the
approved affected GREEN/checks, and normal publication. No MERGE lease.

### Joined scoped viewport RED

Consumer-only run handle 7792 actually joined exit 1, 11.88 s, log
`/tmp/kandev-child40-red-consumer-viewport.log`. The scoped getters expose the
real cards. Both direct switch and close/different reopen fail causally at
Beta's active store retaining `alpha-session`; the primary-metadata control
passes. Three cases execute, no setup/teardown errors. Existing caught plural
HTTP404 logs remain honest. No wider fixture change was needed. The prior
meaningful hook RED is retained rather than replayed. Production correction
and the exact approved two-suite GREEN are next.

## Local implementation results

The correction changes only the existing hook's local snapshot and render-time
projection. Requests retain per-effect cleanup, the 10000 ms transport timeout,
first-list fallback, and store marked-primary-or-first priority. Same-task
close/reopen retains settled fallback while a new read is pending. No consumer
production, store, API, layout, schema, package, or backend edits.

- Exact two-suite GREEN 54386 actually joined exit 0, 14.08 s, 17 tests PASS.
- Prettier completed exit 0 on the three TS/TSX files.
- Changed ESLint 12415 joined exit 1 (five warnings); 26587 joined exit 1
  (one remaining duplicate literal warning). Routine test constants and ternary
  corrections addressed those findings. Final 2570 joined exit 0.
- Approved typecheck 73307 joined exit 0; normal ignored generators only.
- i18n check 34154 and ratchet 33851 both joined exit 0, no new product copy.
- After those fixture-only lint corrections, exact affected GREEN 87620 joined
  exit 0, 13.09 s: both suites execute, 17 PASS. This supplies current real
  active-store and URL isolation, current Beta settlement, metadata precedence,
  plus the 14 hook contract cases. No browser/full-suite replay.

Logs: `/tmp/kandev-child40-green.log`, `-green-final.log`, `-eslint.log`,
`-eslint-corrected.log`, `-eslint-final.log`, `-typecheck-final.log`,
`-i18n-check.log`, `-i18n-ratchet.log`. Earlier failure receipts remain retained.
All local handles are actually joined. Task 01 remains in progress until actual
delivery gates and owned-handle joins finish. LOCAL-HEAVY is exclusively held
through normal publication, then returned. No MERGE lease; no claim of completed
delivery from local success or PR publication. ROOT proof remains untouched.

### Documentation validation before publication

Catalog validation exit 0 (351 decisions, 1,368 specs), all-spec lint exit 0,
public-doc validator exit 0 (47 pages), and `git diff --check` exit 0. Actual
seven-path PR documentation-reference coverage reports `covered`, errors empty,
with the changed runtime hook linked to this one work order and the owning
requirement/design. All command handles terminal. Public docs need no content
change; this restores the documented behavior without changing UI composition,
copy, commands, or settings. State-only mobile exception needs no screenshots
or browser/E2E runs. Normal active hooks and ready publication are next;
hosted gates and ROOT's separate merge lease remain pending.
