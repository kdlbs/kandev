---
created: 2026-10-06
status: complete
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
legacy_specs: []
---

# Implementation Plan: Session File-Review Reader Ownership

## Overview

Keep each mounted reader's map and loading tied to its committed session
lifetime. One sequential work order supplies faithful RED, the local hook fix,
current behavior controls, and bounded validation after a later ROOT release.
The design checkpoint left the four documents unstaged/uncommitted. ROOT reviewed
the package and released implementation in this same primary on 2026-10-06.

UI's existing navigation contract owns stale client publication, independently
of task-owned review storage and APIs. The catalog/source/consumer audit found
no existing dedicated reader requirement. Extend the current contract's `.5`
isolation outcome with only `.12` through `.14`; do not create an incident
requirement, migrate review persistence, or change sibling delivery statuses.
The related [task-session fallback package](../task-session-fallback-ownership/plan.md)
owns a different hook and remains outside this work order.

## Accepted evidence and source checkpoint

Read-only compatibility audit on 2026-10-06:

- Checkout HEAD: `d62824ad2895616e1b97f876c9546786565f61d1`.
- Hook blob: `b34f3e09d713a6e3856ee4aac8e30ef5a1faeca0`, identical to ROOT's
  proofbase `8f533fc85a1b5f0658bae75905fc64ff80c13a87`.
- Archive: `/tmp/kandev-session-file-review-reader-repro.test.tsx`, SHA-256
  `2be78ff6746b4371010b9eb3dd68a7b43f72a15d70eef9ee97fb9d468adf0532`.
- Receipts: `/tmp/kandev-root-session-file-review-proof-receipt.json` and
  `/tmp/kandev-root-session-file-review-proof-classification.json`; raw log:
  `/tmp/kandev-root-session-file-review-proof.log`.
- ROOT handle `38170` actually joined exit 1 in 4.824 seconds: two causal
  failures, one positive control, no setup/timeout failure. Its classification
  includes `not_admitted: true`; this task's prompt explicitly accepts the
  evidence for design only. It is historical evidence, not permanent RED.

An outstanding Alpha request captures unguarded local setters. After committed
Beta and its unreviewed reply, late Alpha publishes its reviewed map directly
into Beta. The identical path and actual hash defeat stale-diff mitigation.
A completed Alpha map also survives committed null. Deferred loading/cache-hit
work and finalization have the same missing lifetime boundary. Recheck relevant
source at later admission; changed source checkpoints ROOT, never silently
replay, rebase, or manufacture equivalent evidence.

## Scope and technical approach

Follow the [mounted reader design](../../specs/ui/system-design/task-navigation-responsiveness.md#mounted-file-review-reader-ownership).
Own `apps/web/hooks/use-session-file-reviews.ts` and one adjacent `.test.tsx`
suite. Use a session/lifetime-tagged snapshot, pure render projection, and
commit-phase token retirement. Guard local publication while preserving shared
cache writes and notifications. Keep fetched reuse and current optimistic
controls. A retired A response can populate A's cache and notify a new A reader;
it cannot use the old local publisher. Reused pending readers remain idle as
today until their cache notification, without a new in-flight registry.

The suite includes a real mounted `ReviewDialog` for the concrete checkbox
outcome where practical. The hook sits above its keyed inner row. Tablet
`TaskCenterPanel` and ordinary task panel classification provide an independently
audited retained-reader path; Dockview disposal and null dialog unmount mitigate
some lifetimes. Do not claim all consumers leak. Only causally necessary immediate
consumer test glue is admissible; no production consumer change is indicated.

Exclude mutation admission/ordering/rollback redesign, read-versus-mutation
ordering, module cache redesign/eviction, transport abort, auth-generation
migration, new public revision policy, API/result shapes, classifier changes,
layout/copy/touch/breakpoints, unrelated candidate fixes, and speculative ADRs.
We are not alone in the repository; preserve other edits and all foreign
checkouts, refs, caches, volumes, and ROOT evidence.

## Tests

All new tests belong to `hooks/use-session-file-reviews.test.tsx`, with the
anchored suite prefix `session file review reader ownership`.

| Criteria | Evidence |
| --- | --- |
| `.5`, `.12` | Pending Alpha to committed Beta with same file/hash; settle Beta false then Alpha true. Real classifier stays unreviewed. Mounted real dialog checkbox/progress remains Beta-owned. |
| `.12`, `.13` | Settled Alpha to null; uncached Beta; pending-to-cache-hit; queued loading/cache-hit then committed retirement; obsolete success/error/finalization; A-to-B-to-A and unmount/StrictMode. |
| `.13`, `.14` | Current success/empty/error/no-client, pending and settled loading, cache reuse/coalescing, same-session multiple readers, other-session notifications, mark/unmark/reset and existing mark rollback. |

Use the real providers, hook, WebSocketClient and product classifier; only wire
transport is deferred. RED must fail by actual state/classification assertion,
not missing methods, setup, timeout, or a replicated predicate. Join and restore
every test-owned resource. ROOT's proof does not establish mutation-order bugs.

## Mobile, E2E and public documentation

Mobile Changes and Review use the same data paths through `MobileChangesPanel`
and `SessionMobileReviewDialog`. This is pure state/data normalization without
rendered structure, copy, navigation, scrolling, touch or breakpoint changes.
The mobile-parity exception uses targeted hook/component proof and this note;
no ASCII layout preview, browser/build/mobile E2E or full suite is planned.

Public-doc audit: `docs/public/sessions-and-review.md:463` already states review
state is session-owned and diff-hash stale detection. The repair restores that
contract. `docs/public/websocket-api.md` get/update/reset actions remain accurate;
README and screenshot catalog need no changes. Internal docs updated only.
Public validators become necessary only if those public sources actually change.

## Work orders

- [x] [Task 01: Enforce mounted reader ownership](task-01-reader-ownership.md)

Exactly one work order, wave 1, no dependencies; sequential in this primary.

## Resource and delivery checkpoints

Task `42631f9c-f6a4-4eec-8c26-75466fe740a0`, session
`d2d82e45-0f65-4320-8d15-11db26e0446f`; retain the same profile and primary.
ROOT reviewed all four final design files and released implementation after the
actual design WAITING checkpoint. Exclusive LOCAL-HEAVY is currently child47;
merge grant NONE. Retain strictly serial owned handles and terminal receipts.
The live task plan preserves the system marker, user edits, question barrier and
standing delivery gates. Review receipt: `/tmp/kandev-root-child47-design-review.json`.

Later implementation requires ROOT's explicit reviewed-package release and
heavy lease. Use the work order's exact bounded commands, owned process groups,
logs, retained handles and actual joins; no automatic resource retry or passing
replay. Later READY commit/push/PR and exact-head all-terminal review collection
are standing-authorized only after release. Inspect substantive authenticated
CodeRabbit App347564 full/all-file evidence and disposition findings. All six
required checks and Backend/Frontend/E2E parent checks must succeed with fresh
complete errors/threads/governance evidence. ROOT alone grants serial MERGE;
normal expected-head squash, no admin/bypass/self-merge. Verify actual merged
tree/blobs/main, join owned cleanup, then ROOT archives and releases its proof.
Lost mutation calls have no verdict until reconciled; never duplicate them.

## Verification results

Design checks on 2026-10-06, all bounded to 60 seconds and actually joined:

| Command/check | Actual result |
| --- | --- |
| `python3 scripts/list-docs.py validate` | Exit 0; 355 decisions and 1394 specifications validated. |
| `python3 scripts/lint-spec-files.py --all` | Exit 0; all specification files passed. |
| Repository `validateCoverage` over actual changed/untracked paths | `ok: true`, `status: exempt`, `errors: []`, because the actual diff is docs-only. |
| Unchanged repository `validateWorkOrder` over actual four-document closure | Exit 0; accepted owning design/requirement, `errors: []`. Direct read-only invocation, no synthetic triggering path. |
| Scoped `git diff --check` | Exit 0. |

Process receipt: `/tmp/kandev-child47-design-checks.json`; logs:
`/tmp/kandev-child47-design-{catalog,spec-lint,coverage-corrected,diff-check}.log`.
Native handle `68383` actually joined exit 0; every child PID/group in the
receipt was terminal and gone. The first coverage wrapper exited 1 because it
incorrectly required `covered` for a legitimate docs-only `exempt` result;
its log/receipt remains preserved. The affected-only corrected invocation above
passed. No timeout/resource/transport failure, global heavy work, or retry.

Implementation validation on 2026-10-06 (all strictly serial, bounded and joined):

| Invocation | Actual result |
| --- | --- |
| Conditional frozen apps install, pinned pnpm 9.15.9 | Exit 0; dependencies initially absent; one install, lockfile unchanged. |
| Permanent baseline RED | Exit 1; nine failures/seven controls passed. Eight causal ownership assertions; the real dialog initially lacked its VCS provider, a fixture setup error. |
| Real-provider correction, affected mounted dialog RED | Exit 1; actual checkbox expected unchecked, received checked after late Alpha. Real retained dialog/provider/protocol, only wire deferred. |
| Initial GREEN | Exit 0; 16 tests passed. |
| Added current deferred optimistic control | Exit 1; found an implementation capture bug: queued cache initialization overwrote current unmark. Read current cache inside the guarded microtask. |
| Initial affected lint | Exit 1; test describe exceeded function-length limit. Extracted sequential test registration helpers. |
| Initial typecheck | Exit 2; only own TS2740 partial user-settings fixture. Use complete real `defaultState.userSettings` and override auto-mark. |
| Final typed-fixture suite | Exit 0; 17 passed with one worker, including real hash/classifier and retained mounted ReviewDialog. |
| Final affected ESLint | Exit 0; both owned hook/test files, zero warnings. |
| Corrected package typecheck | Exit 0; normal pretypecheck and `tsc --noEmit`. |
| `i18n:check` / `i18n:ratchet` | Exit 0 / 0; existing orphan-key warnings only, no new copy. |

Receipts and logs: `/tmp/kandev-child47-{install,red,red-provider-correction,green,deferred-control-red,lint-initial,typecheck,green-typed-fixture,lint-typed-fixture,typecheck-typed-fixture,i18n-check,i18n-ratchet}.{json,log}`.
Native handles and absolute cutoffs are retained in receipts; every completed
wrapper/owned process group and observed child is gone. The interruption after
original typecheck was reconciled against its terminal receipt and closed native
handle; no duplicate unknown invocation. Original failures remain historical.
Final catalog/spec lint and actual six-path coverage passed: `covered`, `ok: true`,
`errors: []`; direct work-order references also passed. Receipts:
`/tmp/kandev-child47-docs-{catalog,spec-lint,coverage}.{json,log}`. Public
docs already cover the restored contract. Delivery/review/merge remain external
checkpoints; no claim of task completion before verified merge and cleanup.

## Risks

- Session-ID-only guards miss generation retirement and deferred loading.
- Blocking all old cache writes breaks valid A-to-B-to-A/shared-reader reuse.
- Remounting a fixture instead of preserving the real reader hides the defect.
- Source drift, timeout/resource/transport/setup or unknown failures require a
  ROOT checkpoint. Repair only routine owned fixture/CLI/oracle mistakes.
