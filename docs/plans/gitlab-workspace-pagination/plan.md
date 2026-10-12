---
created: 2026-10-06
status: done
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-01.md
legacy_specs: []
---

# Implementation plan: Reset GitLab pages across workspaces

## Overview

Reset the existing GitLab browse hook to page 1 on a workspace change so a
smaller workspace's first-page rows remain accessible. One sequential work
order delivered permanent behavioral RED, the narrow hook correction,
and affected GREEN and static checks after ROOT's later implementation grant.
Local implementation is complete; hosted review, CI, and merge remain pending.

## Ownership and scope

Integrations owns the workspace connection and provider browse contract.
Extend the existing [GitLab requirement](../../specs/integrations/requirements/gitlab-integration.md)
with AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9 and its matching split design;
criteria .5 and .8 supply the connection/browse context. The 3,131-byte
requirement and 24,314-byte part-1 design at the audited baseline have room
for this addition. No separate incident specification, task-status/mobile-only
owner, duplicate requirement, or ADR is needed.

Production ownership is only
`apps/web/components/gitlab/my-gitlab/use-gitlab-search.ts`. Permanent tests
may extend the existing hook suite and add one real-hook/real-consumer
workspace suite. Exclusions: consumer production edits, issue consumer
expansion, milestone/project policy changes, exactly-one-request guarantees,
request coordinators/generations, backend/cache/permissions, other providers,
copy/layout/touch/breakpoint/navigation changes, browser/build/E2E, installs or
product execution during design, delegation/tasks/sessions/model switching.

## Baseline and accepted evidence

ROOT supplied accepted read-only proof from
`f66293552d15d53d1ad20b114a9d0c223722c04e`. The one cheap current-base audit
found HEAD `0235c4f833dccdd4cdbaaa03cfc9b2c1c9b5057f`; intervening clipboard
and GitHub association changes do not touch the owned GitLab sources.
Unchanged Git blobs: hook `8cf1678edf6eb6de210f31fd57db112fe18e64e5`,
MRList `a3793fb9b995938dbae24f47a9b96271c38c1f19`, pagination
`a5a614f89fbada1ea99bbb3d45340c6ee680f1a5`.

The hook's page-reset dependencies omit workspace identity. A page-3 search
in A followed by B with one result requests B page 3, leaving B's page-1 row
hidden. Real MRList renders the empty state and real ResultsPagination renders
no navigation because B has one page. ROOT's original proof had two causal
failures and two passing controls: unchanged same-workspace page 3 with no
extra transport, and an initial small-B page-1 native row link.

Original native session 12211, chunks d3303a/cc55df, actually joined exit 1 in
6.980s at 2026-10-06T14:19:39–46Z. Original receipt/classification/native JSON
and log are `/tmp/kandev-root-gitlab-workspace-pagination-proof-*` and
`/tmp/kandev-root-gitlab-workspace-pagination-proof.log`. The protected
candidate `/tmp/kandev-gitlab-workspace-pagination-candidate.test.tsx` is mode
0400, SHA256 `51770d59a14c4ee660f0f498327fe78a7a2461ef39c7c0c7e0cee5e6a45545e7`.
Do not replay, copy, mutate, or remove it. ROOT reports scratch removed, clean
original checkout and all original PIDs/groups gone. Independent permanent
regressions were authored after the later grant.

## Technical approach and consumers

Add `workspaceId` to the existing reset effect beside preset/customQuery/kind.
Preserve its existing request-sequence checks and workspace visibility stamp.
Effect ordering can invoke old-page and page-1 transports on the new workspace;
test accepted final state and superseded-response rejection without imposing
a transition request-count contract.

| Boundary | Actual consumer/shape | Intended evidence |
| --- | --- | --- |
| MR transport | `searchUserMRs`: workspaceId/filter/customQuery/page/perPage; `mrs`, total_count | Real-hook MR workspace reset and current-page controls |
| Issue transport | `searchUserIssues`: same inputs plus milestone; `issues`, total_count | Same-hook issue reset and routing controls; no issue consumer expansion |
| Page state | `app/gitlab/use-gitlab-page-state.ts` passes workspace and committed filters; owns explicit selection/milestone resets | Existing page-state suite remains in GREEN |
| Page client | `app/gitlab/gitlab-page-client.tsx` selects MRList/IssueList and feeds search state to pagination | Read-only inventory; no page-shell/reload policy change |
| Rendered results | Real MRList/shared rows/native links and ResultsPagination | A page-3 navigation, B first-page link, active page, controls present for A and absent for one-page B |

The GitHub counterpart already resets on workspaceId. This is source-only
positive precedent; no GitHub sweep, tests, source change, or task is in scope.
Existing null payload fallbacks and current empty/failure behavior remain
unchanged. No new transport shape or provider fallback is introduced.

## Tests

Use `components/gitlab/my-gitlab/use-gitlab-search.workspace.test.tsx` for
independent real-hook/provider/consumer fixtures and extend the existing
`use-gitlab-search.test.ts` only for meaningful missing controls. New test names
below identify the executed RED selection and final behavioral coverage.

| Criteria | Executable evidence |
| --- | --- |
| .9 workspace reset; .5 workspace data | `resets MR page for a smaller workspace`; `resets issue page for a smaller workspace`; A -> B -> A reset |
| .9 accessible first page; .8 browse result | `shows the new workspace row through real list and pagination`, asserting actual native href and navigation, not empty text alone |
| .9 ordinary page preservation | `keeps same-workspace page navigation on equal inputs` with no extra request; `shows a first-page row on initial small workspace` current control |
| .9 existing reset policy | Preset, committed custom query, and kind changes at later pages still reset; retain milestone/project baseline tests |
| .9 latest-request/visibility preservation | Late old-workspace and new-workspace old-page success and failure after accepted B page 1; pending B masks A items |
| .9 current state and lifecycle controls | Current empty response, current rejection, refresh after reset and later-page navigation, disabled/no-workspace gates and enable recovery |

Keep production hook/store/rows/pagination real. Partially mock only search API
transport with independent workspace datasets, page slicing and held responses;
do not copy a production predicate or write source-string/tautological tests.
Settle every held response and unmount every provider; no leaked requests/timers.

## Mobile and public documentation assessment

Pure state normalization changes no markup, copy, touch interaction, navigation,
scrolling, or viewport branch. Existing shared result rows/pagination apply on
desktop and phone. Real-consumer integration plus targeted hook tests satisfy
the mobile-parity pure-state exception. No ASCII layout proposal or new browser,
build, desktop/mobile Playwright test is required for this boundary.

The public-guide audit includes `docs/public/**`, root README and screenshot
catalog. `docs/public/integrations.md`'s GitLab connection and browse sections
already describe active-workspace isolation and server pagination in pages of
25; project narrowing is limited to the current page. They make no contradictory
reset or request-count claim. No public-guide, API, configuration, terminology,
or screenshot change is needed. Internal requirement/design/package changes
record this pagination clarification.

## Work orders

- [x] [Task 01: Reset the GitLab workspace page](task-01-reset-workspace-page.md) (local implementation)

Task 01 is sequential in this same primary. The design turn ended before
permanent tests or production changes. ROOT subsequently reviewed the full
four files and exact hashes, released authoring, then granted the exclusive
global local-heavy lease and normal delivery. Task 01 is locally `done`.
Publication, hosted review/CI, and separate ROOT merge authorization are tracked
in the live task plan and remain pending at this local completion checkpoint.

## Verification results

Design checks passed on 2026-10-06: catalog validation (355 decisions and 1,404
specifications), all 36 spec-validator tests, full spec lint, and reference
preflight (`errors: []` for actual four-document inventory, which is exempt;
separately labelled planned-hook trigger is covered). Whitespace check passed.
Full-file/status/SHA256 receipt is retained in the live task plan and external
design receipts at `/tmp/kandev-gitlab-pages-design-5a73bdd1`; hashes are taken
after the final documentation results update.

Independent permanent RED produced three causal failures (MR page, issue page,
and the hidden real MR row) with three passing current controls against the
unchanged hook blob. After the sole dependency edit, all 61 tests in three
affected suites passed, including all 25 new workspace regressions. Changed-file
ESLint, normal project typecheck with its existing pretypecheck generation,
i18n:check and i18n:ratchet passed. Exact commands and bounded process requirements are in
[Task 01](task-01-reset-workspace-page.md#verification).

The final documentation gates and native receipts are recorded in the live task
plan before publication. Hosted review/CI and merge remain pending.
Do not promote the whole unrelated migrated split design based on this repair.
The live task plan preserves identities, system marker, resource receipts,
ROOT-only delivery/merge gates and user edits; it is the operational record.

## Risks

- A transient old-page request is expected under existing effect order. Fixtures
  must allow it and defer responses by workspace/page rather than rely on call
  order or enforce duplicate suppression.
- Pagination display clamping cannot prove the page-1 transport happened. Assert
  both accepted hook results and the real native B row link.
- Real consumers need the actual provider, Tooltip setup and locale catalogs.
  A setup/resource/transport failure is not a behavioral RED verdict.
- Original proof remains read-only. Any out-of-scope requirement or runtime
  failure checkpoints ROOT; no automatic retry or expanded architecture.
