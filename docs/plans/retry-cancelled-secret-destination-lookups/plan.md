---
created: 2026-10-06
status: implemented
requirements:
  - REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001
system_design:
  - ../../specs/workspaces/system-design/secret-scope-transfer.md
legacy_specs: []
---

# Implementation Plan: Retry cancelled secret destination lookups

## Overview

Restore destination-name pre-check recovery when a mounted Workspace-source Copy/Move dialog leaves a pending workspace destination for General and returns. One sequential work order corrects the hook-local cache lifecycle and proves the existing dialog conflict outcome using real production dependencies and deferred fetch transport.

Workspaces owns this capability because it owns secret scope, destination identity, and transfer semantics. The existing requirement/design pair is extended minimally; no incident requirement, ownership migration, ADR, or UI-owned duplicate is needed. The original [secret-scope-transfer package](../secret-scope-transfer/plan.md), especially its completed frontend and E2E work orders, remains a historical delivery record; this repair does not reopen its backend, extraction, locale, or geometry work.

## Admission and evidence

- Admitted checkout HEAD: `6d34f653f82927874fcab7cd63921a185a22877e`; actual own-source audit read the hook and rendered consumer. No main-only rebase is authorized.
- Hook blob: `338748cc59df9f55cd31ff4a9b157af7e44cca13`; requirement/design admission blobs: `46df777fe5055e263b561c0f56049f28b6ae3e2f` / `0fe1bb082945d6a3d64012eda3e6a0815efc7056`. All three match accepted proof base `5010081464673fb41c163c06a64926308cb15d43`.
- Accepted read-only archive: `/tmp/kandev-secret-destination-pending-cache-repro.test.tsx`, SHA256 `7f9fe2954aa0a03cac0de44162b3579a41374a0b5f974b771d5c0e70474d3a77`. Receipts: `/tmp/kandev-root-secret-destination-proof-receipt.json`, `/tmp/kandev-root-secret-destination-proof-classification.json`, and `/tmp/kandev-root-secret-destination-current-main-audit-20261006T0603Z.json`.
- ROOT actual handle 96117 joined exit 1 in 5.225s: two causal missing replacement-fetch failures (pending scope roundtrip and standalone hook StrictMode), three passing completed-cache/refreshKey/workspace controls. No setup or timeout failure. Preserve the archive and receipts without replay, deletion, or edits.
- Current root cause: `cacheKeyRef` is marked at admission; effect cleanup cancels publication but leaves that key reusable. Returning skips the replacement fetch.
- Qualification: the dialog starts at null and auto-selects, so standalone StrictMode RED is not evidence every initial dialog mount fails. The hook proof establishes neither transfer corruption nor a disabled-submit bug. The faithful rendered regression is still required after implementation release.

## Scope

Own only the local hook correction, two new production regression suites, and the four design artifacts. Preserve completed same-session reuse, explicit refreshKey invalidation, changed-workspace publication isolation, Global store reads, current lookup failure fallback, API options and transfer payloads.

Exclude backend, API/schema/types changes, shared caches, abort frameworks, loading restrictions, new copy/layout/navigation/breakpoints, browser/E2E/build/backend/DB/broad-suite runs, and optional cleanup. Production consumer glue requires new causal evidence and a ROOT scope checkpoint; it is not in this work order.

## Technical approach and inventory

| Boundary | Actual files and responsibility |
| --- | --- |
| Owned hook | `apps/web/hooks/domains/settings/use-secret-destination-names.ts`: cache marker only becomes reusable after current settlement; canceled reads never finalize it. |
| Consumer | `apps/web/components/settings/copy-move-secret-dialog.tsx`: destination/null/auto-select, refresh key, existing conflict and transfer handlers. `copy-move-dialog-body.tsx`: real selector, name invalidity/message and primary action. Read-only production dependencies. |
| State | `apps/web/components/state-provider.tsx` instantiates `createAppStore` in `apps/web/lib/state/store.ts`; hydrate workspaces and Global secret metadata via actual provider/store APIs. |
| Transport/types | `apps/web/lib/api/domains/secrets-api.ts`, `apps/web/lib/api/client.ts`, `apps/web/lib/types/http-secrets.ts`: actual GET scoped query, no-store, JSON Response decoding and metadata shape. No substitutions or edits. |
| Destination availability | `apps/web/hooks/domains/settings/use-workspace-destinations.ts`: preload actual workspace state to avoid unrelated fallback fetch. |
| Legacy tests | `use-secret-destination-names.test.ts`, `use-workspace-destinations.test.ts`, `copy-move-secret-dialog.test.tsx`, `lib/api/domains/secrets-api.test.ts`: existing focused controls use module fakes; retain them but do not use them as the new causal proof. |
| New suites | `apps/web/hooks/domains/settings/use-secret-destination-names.lifetime.test.tsx`; `apps/web/components/settings/copy-move-secret-dialog.destination-names.test.tsx`. Both use only a deferred global fetch seam. |

Follow the owning design's [lifecycle](../../specs/workspaces/system-design/secret-scope-transfer.md#destination-name-lookup-lifecycle). On admitting a workspace read invalidate prior reuse, then promote the current result only at accepted success/failure settlement. Keep existing cancellation fencing. No exported private helpers for tests.

## Test mapping

| Criteria | Permanent evidence planned after release |
| --- | --- |
| AC 001.9 | Hook pending workspace -> Global -> same workspace replacement, stale success/failure in either settlement order; rendered Workspace-source picker workspace B -> General -> B while pending, resolve fresh duplicate, assert `aria-invalid`, associated existing message, disabled action, then rename and observe enabled action. |
| AC 001.10 | Completed success roundtrip reuse and same-key rerender; refreshKey refetch; workspace A -> B with late A ignored; Global-store names; standalone actual StrictMode setup/cleanup replacement. Rendered completed-cache roundtrip and reopen/new-session refresh controls. |
| AC 001.11 | Current fetch rejection/HTTP failure yields loaded empty names; canceled rejection cannot finalize current read. Rendered current failure allows otherwise valid submission; actual transfer 409 and non-409 handling remain intact through deferred HTTP Responses. |

Fixtures must clean up rendered trees, settle every owned deferred fetch, drain completion using `act`, and restore globals. No sleeps, enlarged timeouts, assertion weakening, module mocks, fake duplicate dialog, or substituted API/predicate/hook/store. Default Radix events and accessible labels are exercised. All tests use complete secret metadata, hydrated source/destination workspaces, and real locale setup.

## Mobile and public-doc audit

Pure shared request state/data correction inside the current component satisfies the mobile-parity exception: no rendered structure, touch, scrolling, navigation, or viewport-dependent behavior changes. The rendered consumer test checks the conflict projection shared by both viewports. No new ASCII layout or mobile Playwright test is needed. Existing `apps/web/e2e/tests/settings/secrets-copy-move.spec.ts` and `mobile-secrets-copy-move.spec.ts` describe the original transfer and phone surface, not proof of this canceled-roundtrip repair; they are not run here.

Read `docs/public/README.md`; searched `docs/public`, root `README.md`, and `docs/screenshots.md`. `docs/public/agents-and-profiles.md` already describes Copy/Move and duplicate-name blocking (how-to audience); this correction restores that outcome without changing commands, terms, routes, payloads, screenshots, or instructions. No public text update is needed. Owning specs/plans change, and public validation still runs as a lightweight gate. ADR 2026-08-03 scope/merge ownership remains unchanged.

## Work orders

- [x] [Task 01: Restore canceled destination lookup recovery](task-01-restore-destination-lookup.md) (`done`, sequential, no dependencies).

## Design checkpoint and later delivery barriers

This turn ends after exactly four unstaged/uncommitted artifacts and lightweight doc checks. Task `30ff8f34-29a2-4dac-9bfa-6e2cf258bba7`, session `52a0445b-19d3-4feb-b900-6eb342310e4a`, title ownership and system marker are retained in the Kandev task plan. No delegation/tasks/sessions/tabs/model switches. Global LOCAL-HEAVY and MERGE: NONE. ROOT must review actual files, then issue a later SAME-primary implementation interrupt identifying the exclusive global LOCAL-HEAVY release. Design status or a checkbox is not implementation admission.

After that release, execute Task 01's scoped gates, normal active commit hooks, one new Conventional Commit (no amend/bypass), push, and a ready PR. Freeze published head except a real corrective finding; no moving-main-only rebase, synthetic merged tests, or optional polish. Return LOCAL-HEAVY only after every local/publication command is actually joined, all own PIDs/groups are gone, worktree clean, exact remote verified, and all owned blobs recorded.

Use one initial normal all-terminal 45-minute CI collector, retaining actual handle/PID/group/start/deadline/watchdog/log. Never duplicate timer GitHub queries. Join it and verify its PID gone before ROOT authorizes a specific replacement. No CI reruns without a ROOT workflow/job-NAME budget grant; no attempt/ID reset or whole/passing reruns. Early during CI, inspect authenticated configured CodeRabbit App 347564 automatic substantive FULL review of exact head and ALL actual files; progress/ACK is not passing review. One necessary full request is allowed only for an actual completed skip/gap; no redundant optional-review wait. Ground every finding/disposition.

END MERGE-ready requires all six actual required checks SUCCESS, actual Backend/Frontend/E2E parents SUCCESS, fresh complete error-free exact-head state/resolver/governance, zero visible/hidden actionable changes-requested/human gates, and all joins. Merge separately requires ROOT's serial grant for normal expected-head squash (no admin), actual merged SHA/content/remote verification and owned-only cleanup joins; ROOT independently verifies/archive/checksums the original proof and refills work. Publication, timeout, or a plan checkbox never completes the task.

Callback/question queue is full: persist critical barriers in task plan/conversation and END WAITING; do not retry notifications/questions or invent approvals/leases. Preserve managed worktree/dependencies/shared caches, foreign resources, paused giant work, and unproved volumes. Machine crash absence cannot be guaranteed.

## Verification results

Design documentation checks passed on 2026-10-06: public validator tests 62/62, 47 published pages, catalog 351 decisions/1369 specifications, spec-validator tests 36/36, full spec lint, catalog discovery of the owning pair, local documentation coverage preflight (four documentation paths are exempt), actual four-file inventory/local document links/untracked whitespace, and git diff whitespace. The exempt preflight is not a traceability verdict; direct frontmatter/REQ/AC/design checks are recorded separately at handoff.

Actual retained tool handle 38918 joined exit 0; wrapper PID 796415 and all nine gate process groups were gone at join. Receipt `/tmp/kandev-child45-design-1791267705902181086.json` records upfront PID/group/start/cutoff/log and actual joins. Implementation and product tests remain pending: no admission. No install, production/permanent-test change, staging, commit, push, or PR occurred.

Implementation passed its scoped gates after later ROOT release. See [Task 01 results](task-01-restore-destination-lookup.md#results) for actual RED/GREEN history, fixture corrections, command receipts and counts. The initial six-suite run passed all 34 existing tests; final affected new-suite run passed 17/17. Resource caps were unchanged. Normal publication and hosted checks/review/merge remain pending and are not implied by `implemented`.

## Risks

An admission-only or success-only marker fix can break completed failure fallback or cache reuse. A stale finalizer can reinstate an abandoned key. A mocked-hook dialog test can hide the defect. Resolve these with the work order's real production fixtures and current/canceled settlement controls. Resource, timeout, transport, unknown failure, or scope expansion checkpoints ROOT; no automatic recovery, memory/timeout increase, cache wipe, or foreign-process kill.
