---
created: 2026-10-06
status: implemented
requirements:
  - REQ-UI-EMBEDDED-EDITOR-TARGET-001
system_design:
  - ../../specs/ui/system-design/embedded-editor-file-targets.md
legacy_specs: []
---

# Plan: Embedded editor file targets

## Outcome and admission

Preserve the backend-resolved file identity and effective coordinates through
the existing embedded-editor sentinel consumer to the real `vscode.openFile`
wire. Deliver producer, consumer and regression controls together in exactly
one sequential work order, after a later explicit ROOT implementation interrupt.

Design ended before ROOT reviewed all four documents and explicitly released
implementation in the same primary. Task01 is implemented and scoped behavioral
checks pass. ROOT granted child48 sole GLOBAL LOCAL-HEAVY for remaining local
checks and publication; MERGE NONE. No proof replay, browser/build/E2E/native
launch, delegation, recursive tasks, additional sessions/tabs or model switch.

Task `efe59569-d0f1-463a-87b3-0096cc02da5a`; session
`00eb41cd-49d6-48f3-9775-a79b4537f7e4`. ROOT continuous-loop child48.
Callback/question queue is full: record durable checkpoints and END WAITING;
ROOT watches directly. Preserve the Kandev plan's system marker, user edits,
identity and version checks. End the design turn before implementation can start.

## Grounding and accepted proof

Read-only audit HEAD: `9885f0e558eb0dfd763e5f2106caadf43975d20d`.
Current hook blob `6ac8060bf76826028197f58c854cdca3ca9189f7` and service blob
`ba2c7d8e5db1fc9ffef03b1475c09add363b8dd0` match ROOT's independently verified
main audit. No reset, rebase, replay, or dependency installation occurred.

Accepted evidence belongs to ROOT, not this child: source proofbase
`d62824ad2895616e1b97f876c9546786565f61d1`, native handle `64968`, actually joined
exit 1 in 6.031s. Two causal failures and one ordinary positive; no setup or
timeout failure. Read-only artifacts:

- `/tmp/kandev-embedded-editor-colon-path-repro.test.tsx`, SHA256
  `c4e6ff9001008b351a5ff4e757a6c35cd2fcebcae6567e797f0e4644ece5ceed`.
- `/tmp/kandev-root-embedded-editor-colon-proof-receipt.json`.
- `/tmp/kandev-root-embedded-editor-colon-proof-classification.json`.
- `/tmp/kandev-root-embedded-editor-colon-proof.log`.

The real hook/request/toast/API/`fetchJson`/`WebSocketClient` chain received
`(src/name,0,17)` instead of `(src/name:part.ts,17,5)` and `(notes,2026,0)`
instead of `(notes:2026,0,0)`. `(src/plain.ts,17,5)` passed. Only HTTP/socket
wire was substituted. The supplied sentinel mirrored the inspected producer;
this proof did not execute Go or native VSCode. Null Dockview made panel opening
a no-op. Preserve all proof files unchanged and never replay them here.

Cause: `buildInternalVscodeURL` appends coordinates to a raw filename;
`parseInternalVscodeURL` splits every colon from the left. Splitting from the
right cannot disambiguate numeric-colon filenames without coordinates.

## Ownership, scope and approach

The [owning requirement](../../specs/ui/requirements/embedded-editor-file-targets.md)
defines reusable embedded-editor targeting. Existing folder-opening,
availability and file-tree-state criteria have distinct contracts and are not
extended into exact-file targeting. No existing exact-target pair was found.

Change only the private producer/consumer serialization: Go query-encodes
path-only `goto` plus independent `line`/`column`; frontend reads query values
once. Keep backend cleaning/selected-worktree resolution authoritative. Retain
the current HTTP DTO, `{url}` response and WebSocket payload; no public fields.
Details and audited router/client/CLI flow are in the
[system design](../../specs/ui/system-design/embedded-editor-file-targets.md).

| Surface | Contract retained | Planned evidence / limit |
| --- | --- | --- |
| Embedded file, supported POSIX target | Exact canonical path and effective cursor tuple | Actual Go service + real hook to socket wire |
| No-file/root or marked directory | Existing panel action, no file dispatch | Bare/directory controls; no rendering claim |
| Selected/default worktree | Existing resolver and canonical relative path | Go selection/fallback/errors and POST forwarding; no new runtime-root mapping |
| External hosted/SSH URL or command editor | Existing URL/launch contract | Existing Go URL/args tests and frontend external response control; no native launch |
| Unavailable/invalid session/editor/worktree | Existing rejection and feedback | Service errors and real HTTP error control; no capability redesign |
| Native Remote CLI | Existing `--goto` construction and admission | Read-only audit only; numeric-colon CLI interpretation remains a residual |

Exclude generic URI/security frameworks, registry/settings/executor redesign,
session request/toast lifetime, concurrency/cache/transport changes, dot-prefix
path guard repair, universal Windows support and blanket test skips. No ADR:
this local private correction is fully explained by its owning pair and tests.

## Tests and responsive behavior

The [work order](task-01-preserve-file-target.md) defines exact RED/GREEN
selectors and controls. Add shared JSON oracle plus service integration tests;
frontend tests use that same oracle through real providers/API/protocol with
transport-only stubs. The oracle is an expected contract, not copied parsing or
an assertion against source text. No missing-method/setup/timeout RED is valid.

| AC suffix | Permanent behavioral evidence |
| --- | --- |
| 001.1, 001.2 | `TestOpenEditor_EmbeddedFileTargets`; `embedded editor target wire` colon/numeric/ordinary/line-only rows |
| 001.3 | Same shared rows: percent sequences, Unicode, spaces, plus and query punctuation |
| 001.4 | Service bare/root controls; real hook no-file and marked-directory controls |
| 001.5 | Service canonicalization/selection/fallback/errors; real POST, missing-session, HTTP-error, external-editor controls |
| 001.6 | Shared hook tests with no viewport branch or rendered changes |

Suffixes refer to `AC-UI-EMBEDDED-EDITOR-TARGET-001`. Tests and fixture paths are
specified in the order. Producer assertions compare real service output to the
same response oracle consumed by the frontend; assert exact request tuples and
results at the protocol boundary.

No rendered composition, copy, navigation, touch, scrolling or breakpoint
change: mobile-parity's pure state/data exception applies. No ASCII UI preview
or new mobile Playwright test. No browser/build/E2E/native runtime proof is
required for this bounded dispatch contract or claimed by this package.

## Documentation audit

`docs/public/developer-tools.md` (how-to), `docs/public/websocket-api.md`
(reference), root README and screenshot catalog were searched. Existing
developer-tools worktree guidance and the documented `vscode.openFile` action
remain accurate; none documents the private sentinel grammar. No public docs,
labels, screenshots, commands, configuration or API fields change. Only this
internal pair and delivery package need updates; recheck if scope changes.

## Work orders and checks

- [x] [Task 01: Preserve the file target](task-01-preserve-file-target.md)

One wave, no dependencies, sequential. The order owns bounded TDD, scoped lint,
frontend typecheck/i18n and actual changed-path documentation coverage.

Design-light gates are capped at 60s per invocation and actually joined:
catalog validation, spec-linter tests, full spec lint, whitespace, catalog
discovery, package references and actual changed-path coverage. For four docs
alone the coverage helper must report `exempt`, with no triggering paths; do
not substitute fake code paths to claim `covered`. Independently check the
actual four files' REQ/AC/design/manifest links because exemption does not
validate them. Gate receipts/logs belong under `/tmp`, not additional repo docs.

## Verification results

At the design review checkpoint, implementation was pending. Six design-light
gates passed on 2026-10-06:
catalog validation, spec-linter tests, full spec lint, whitespace, discovery and
independent package-reference/actual changed-path coverage checks. Each command
had an upfront 60s cutoff and log; all child groups were actually joined and
gone. Native handle `44954` actually joined exit 0 (completion chunk `a339f4`);
wrapper PID/group `1414686` was verified gone. Receipts:
`/tmp/kandev-child48-design-light-receipt.json`; logs:
`/tmp/kandev-child48-design-light-<check>.log`.

The actual four changed paths were documentation-only: helper result `exempt`,
`errors=[]`, `triggeringPaths=[]`, `acceptedReferences=[]`. No code trigger was
simulated and no `covered` verdict is claimed. Independent reference checks
validated all six AC IDs, shared owning REQ/design, manifest/order links, exactly
one sequential order, current package scripts/source symbols and whitespace in
untracked files. Planned new tests/fixture remain absent. Both new specs appeared
in the actual UI catalog. All four files remain below applicable size ceilings.

Node `v24.21.0`, pnpm `9.15.9` and Go `go1.26.0`
were found under `/home/jcfs/.local/share/mise/installs`; Node/pnpm are absent
from the shell PATH. `apps/node_modules` and `apps/web/node_modules` are absent.
No install had run at design review. Later conditional installation is exactly one frozen apps
install under ROOT's heavy lease, as specified by the work order.

### Implementation checkpoint: authoring while the heavy lease is returned

ROOT reviewed all four documents and released Task01 implementation in this
same primary; acceptance is `/tmp/kandev-root-child48-design-review.json`.
Task01 is in progress. The conditional frozen apps install ran once with pinned
pnpm9.15.9 and Node24: native63447 actually joined exit0 (chunk018bbf),
wrapper/group1425877, child/group1425879 and observed descendants gone.
Receipt: `/tmp/kandev-child48-install-1-receipt.json`. Dependencies are present;
do not repeat installation.

The real Go producer/service permanent RED ran once before production changes:
native27365 actually joined exit1 (chunkcb8eaa), wrapper/group1429706,
child/group1429709 and observed descendants gone. All twelve shared fixture
rows failed on actual `OpenEditor` response versus the lossless target oracle,
without compilation, setup or timeout failure. Receipt/log:
`/tmp/kandev-child48-red-go-1-receipt.json` and
`/tmp/kandev-child48-red-go-1.log`. Do not replay this RED or the ROOT proof.

ROOT then requested a bounded lease yield for child47 correction. LOCAL-HEAVY
was explicitly returned after both original handles actually joined and all
owned groups/observed descendants were verified gone. LOCAL-HEAVY NONE;
MERGE NONE. No current heavy invocation exists. The subsequent narrow
continuation admits reads, permanent frontend test authoring and this existing
package checkpoint only. The authored wire test uses the twelve shared JSON
responses through the real hook/request/provider/API/socket client and exact
HTTP/RPC assertions, plus seven workspace/session/error/external controls.
Only HTTP/socket wires are substituted; external opening is observed with a
call-through spy. Teardown drains timers and restores globals/client. This test
has not run: authoring is neither RED nor PASS evidence.

Exact dirty inventory is seven untracked, unstaged files: this manifest and
Task01, the owning requirement/design pair, the Go test, its JSON fixture and
the frontend wire test. Production sources and accepted ROOT proof remain
unchanged. No production edit, frontend run, commit/push/PR or collector.
END WAITING for an explicit same-primary ROOT heavy continuation. Next admitted
heavy action will be the work order's bounded permanent frontend RED; only
after that causal evidence may the producer/consumer correction begin.

## Execution and delivery constraints after review

ROOT alone grants one global LOCAL-HEAVY lease. Use serial invocations with
upfront argv/cwd/start/absolute cutoff/log and wrapper/child/group/native-handle
receipts; actually join and verify all owned processes gone before another run.
The order contains budgets. Do not automatically retry resource/timeout,
transport, unknown or out-of-scope failures, or replay passing checks. After
release, causal own fixture/CLI/lint/oracle corrections may rerun affected checks
only. Protect foreign caches/processes/refs/worktrees/deps, paused oversized
work and the unproved volume; the primary is not alone in the codebase.

Normal active hooks and Conventional Commit; formatter failure means restage
and a NEW commit, never amend/bypass. Standing authorization later covers one
normal push and READY PR. Reconcile remote SHA/existing PR before repeating an
unknown mutation. Return LOCAL-HEAVY with actual joined/gone proof for all
current invocations and exact publication evidence before one original
45-minute all-terminal collector (GNU timeout 46m, kill-after 10s). Preserve
original native handle/PID/group/start/cutoff/log; no replacement or hosted rerun.

Freeze published SHA except a grounded corrective finding. No main-only rebase,
synthetic merged tests, weakened gates or optional polish. Configured
CodeRabbitApp `347564` must provide substantive FULL exact-head review of ALL
files; automatic review suffices. Inspect actual skip/gap before at most one
necessary full request. No ACK/progress/optional reviewer wait.

Require six required checks and actual Backend/Frontend/E2E parent SUCCESS,
fresh complete `errors[]`, resolver/nohidden/actionable/human/changesrequested/
governance evidence and all joins. If a real backend-code PR fixup occurs, run
the full changed lint at exact PR base with the order's bounded payload before
pushing. MERGE-ready END WAITING requires a separate ROOT serial expected-head
normal squash grant; never self-merge/archive/create next child. Completion
requires independently verified merge/tree/all owned blobs/main and joined
only-owned cleanup; ROOT owns archival, original proof release and refill.
Lost invocation stays NO VERDICT until actual state reconciliation.

## Risks and handoff

Mixed backend/web sentinel versions are not compatible; refresh stale bundles.
Native Remote CLI numeric-colon interpretation and existing worktree/runtime
root correspondence are not established by dispatch evidence. Current
fire-and-forget error behavior stays intact. No material design question remains.
End at the four-document review checkpoint and wait for a later ROOT interrupt.

## Implemented behavior and local conformance

The real Go resolver now query-encodes its resolved path in `goto`, with
positive `line` and `column` in separate values. The shared hook reads
`URL.searchParams` once and dispatches that returned path with its coordinates.
Existing session/worktree admission, canonical resolution, directory policy,
external editors and fire-and-forget errors are unchanged. Exactly two small
production functions changed; no public schema or runtime framework changed.

Permanent frontend RED native88878 actually joined exit1: twelve target rows
failed causally (five absent RPCs due to query order, seven wrong path/coordinate
tuples); seven controls passed. No setup/timeout failure. After the correction,
Go GREEN native74287 joined0, and frontend GREEN native81114 joined0: both
suites, 25 tests passed. Scoped Go lint40994 and ESLint66143 joined0; web
typecheck90758 joined0 in71.828s including generators; i18n check66175 and
ratchet25303 joined0. All original wrappers/groups/observed descendants were
verified gone before each next invocation. Receipts and logs are under
`/tmp/kandev-child48-{red-frontend-1,green-go-1,green-frontend-1,lint-go-1,lint-web-1,typecheck-1,i18n-check-1,i18n-ratchet-1}-receipt.json`
and corresponding `.log` paths. Exact argv, cwd, upfront UTC cutoff, memory
limits, original native handles and joined/gone results remain in those receipts.

All six ACs conform at the service-to-wire boundary: exact shared targets and
single decode, positive coordinates, no-file/folder suppression, rejection and
selection/external controls, and the common hook's pure data behavior. Native
CLI rendering and secondary-worktree runtime-root correspondence remain unproved;
mixed bundles still require refresh. Public docs need no change because they
do not expose this private grammar. No screenshots or mobile E2E are required
for this pure data correction. Documentation gates and normal hook/publication
evidence follow separately; local conformance does not authorize merging.
