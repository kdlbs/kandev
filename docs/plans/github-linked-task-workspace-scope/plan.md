---
created: 2026-10-06
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001
system_design:
  - ../../specs/integrations/system-design/github-pr-task-association-reads.md
legacy_specs: []
---

# Implementation Plan: GitHub Linked Task Workspace Scope

## Overview

Make the GitHub PR-to-task reverse read respect the requested workspace and
current context generation. One sequential work order corrects the existing
hook and proves the result through the real store and row indicator. This
package completed implementation and local verification in the same primary
session after ROOT reviewed the design and granted exclusive local-heavy
ownership. Publication, hosted verification and merge remain external gates.

## Scope

### In scope

- [Scoped reverse reads](../../specs/integrations/requirements/github-pr-task-association-reads.md),
  AC .1 through .7, and their actual linked-task controls.
- Reactive reads of existing context/cache stamps, without changing consumers.
- Compatible grouping fixtures and immediate hook-to-row regression tests.
- Owning specification lifecycle and task-defined verification evidence.

### Out of scope

- `useWorkspacePRs` workspace-only `fetchedRef` refresh scheduling, transport
  coalescing, retry/recovery guarantees and global caches or context frameworks.
- Backend, permissions, settings, revisions, reconciliation, unlink, PR key
  normalization, GitLab or issue sweeps.
- Consumer production edits without new causal evidence; layout/copy/navigation,
  runtime profiles, dependency declarations, harness or global configuration.
- Browser/build/Playwright checks without a causal need. No installation,
  product checks, permanent tests, commit, push or PR during design.

## Baseline and accepted evidence

Audited HEAD: `f66293552d15d53d1ad20b114a9d0c223722c04e`.
The hook source blob is `d7bc20d5c9f93d35e78822957d32e371aca57d6c`;
`use-task-pr.ts` is `f6aadc305c70b0e72203db20afd784352bdca149`.
At this baseline, `usePRKeyToTasks` traversed unqualified
`taskPRs.byTaskId` and memoized only that collection. Context resets leave the association cache in place.

Accepted original evidence, read without replay:

- `/tmp/kandev-github-pr-reverse-scope-repro.test.tsx`, SHA256
  `94b38a382b47223cf14403f569d7304c039d3ccc567a66afcd3f111e801b743e`.
  Never replay, modify or remove this original.
- `/tmp/kandev-root-github-pr-reverse-scope-proof-receipt.json`,
  `/tmp/kandev-root-github-pr-reverse-scope-proof-classification.json` and
  `/tmp/kandev-root-github-pr-reverse-scope-proof.log`.
- ROOT native `67599`, initial `054bd1`, completion `569201`: actually joined,
  exit 1, 5.844 seconds, four intended failures and two positive controls.
  Failures cover old A links during pending B, obsolete same-workspace generation,
  null request and the real old-task button. Current-scope map/button controls
  pass. The receipt records no setup/resource/transport failure, unchanged
  production, scratch removal, clean ROOT checkout and gone original processes.

These are prior execution receipts, not new test results in this worktree.

## Technical approach

In `apps/web/hooks/domains/github/use-pr-key-to-tasks.ts`, select `taskPRs`,
`workspaces.activeId` and `workspaceContextGeneration`. Gate the memoized
inversion using the four checks in the [design](../../specs/integrations/system-design/github-pr-task-association-reads.md#read-eligibility).
Depend on the requested workspace and selected snapshots; retain the existing
loop, defensive skip, object references, ordering and key format.

Audit confirmed the actual chain: SPA route active workspace → `GitHubPageClient`
→ `AuthenticatedLayout` → `usePRKeyToTasks` → `ResultsList` → `PRListBody` →
`PRRowTaskIndicator` → shared `TaskRowIndicator`. The row already renders its
localized empty state for a missing map entry. No consumer production change is
needed. GitHub settings and the existing workspace-list HTTP endpoint already
take an explicit workspace; they do not cure an unqualified cached read.

| Boundary | Existing shape | Result and evidence |
| --- | --- | --- |
| GitHub browse | HTTP workspace list, stamped store, `owner/repo#number` | Current associations only; real hook/store/deferred transport/row tests |
| Matching boot cache | Existing initialization normalizes compatible metadata | Remains visible while request is pending |
| Missing or obsolete context | Null/mismatched workspace or missing/older generation | Empty map and existing empty row indicator |
| Independent stores | Top-level `StateProvider` instances | Distinct same-key associations; no shared result |
| Other providers/issues | Independent existing hooks | Excluded; no support claim or sweep |

## Mobile and public documentation assessment

The shared row already supplies phone touch targets, wrapped titles, workflow
step text and task navigation. This correction changes only state eligibility;
it changes no layout, copy, scrolling, navigation or viewport-dependent behavior.
The mobile-parity pure-state exception applies: real hook/component tests prove
the same input for both viewports; no ASCII layout redesign or new mobile
Playwright test is required.

Public sources audited: `docs/public/integrations.md`, root `README.md` and
`docs/screenshots.md`. They describe workspace-owned integration data and browse
task associations. This fix restores that expected scope without a new setting,
API, operation, label or screenshot. Internal docs are updated; public docs need
no change for this correction. No broader security/authorization guarantee is
introduced. No ADR is warranted for this local reuse of existing context stamps.

## Tests

The [work order](task-01-scope-reverse-associations.md#regression-matrix) maps
every acceptance criterion to named permanent tests. New tests live in
`apps/web/hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx` and use real
production hooks, provider/store and row components. Existing grouping tests in
`use-pr-key-to-tasks.test.ts` gain valid scope fixtures and a transport-only mock.
The immediate `pr-row-task-indicator.test.tsx` suite remains a compatibility check.

## E2E tests

End-to-end evidence for this slice is the real hook-to-row component integration
with only workspace-list transport deferred or rejected. No browser or E2E run
is planned: the cause is cache read eligibility, and no viewport, navigation or
server behavior changes. Do not claim browser verification from these tests.

## Work orders

- [x] [Task 01: Scope reverse associations](task-01-scope-reverse-associations.md)
  (`done`, implementation and local checks, wave 1, no dependencies, sequential).

## Verification results

Design validation passed on 2026-10-06:

- Catalog validation: 355 decisions and 1,402 specifications; the new pair is
  discoverable under Integrations.
- Specification validator tests: all 36 passed.
- Full specification lint: all files passed.
- Absolute Node24.21.0 `validateCoverage` reference preflight using the four
  actual documents and the planned hook trigger: `covered`, `ok: true`,
  `errors: []`. This proves document linkage, not product execution.
- Whitespace, local document links, exact four-file unstaged/uncommitted
  inventory, unchanged baseline blobs and immutable proof checksum passed.
  The accepted original process IDs and group were independently observed gone.

Native collectors ran serially with 60s TERM/kill10 limits, upfront PID/group,
UTC start/cutoff, argv and log. Catalog `1745547` joined in 0.634s; spec tests
`1745589` in 0.240s; spec lint `1745633` in 0.410s; references `1745992` in
0.038s; inventory `1747368` in 0.675s. Every original exited 0 and its process
group was gone. Logs are `/tmp/kandev-github-scope-design-{catalog,spec-tests,
spec-lint,references,inventory}.log`.

Design ended at 13:36:29.508. ROOT reviewed all four artifact hashes in
`/tmp/kandev-root-child52-design-review-20261006.json` and issued a later
same-primary authoring release. All four hashes matched before status changed.
Permanent regression authoring and compatible grouping fixture adaptation ended
at 13:48:21 without product execution. ROOT then released implementation under
exclusive local-heavy ownership. The [work-order results](task-01-scope-reverse-associations.md#results)
record all execution outcomes, including the intended four-failure RED with a
passing current-context control, final 30-test GREEN, corrected fixture-only
lint/type errors, clean affected ESLint, normal project typecheck, i18n checks,
documentation checks and actual changed-file coverage. Every original native
collector actually joined and its process group was gone.

Task 01 is done, requirement active, design current and this plan implemented
for local delivery. The sole production file is `use-pr-key-to-tasks.ts`;
request scheduling and all excluded boundaries are unchanged. Exact inventory:
one hook, two test files and these four owning documents. The protected original
proof remains unchanged and was never replayed.

Normal active-hook commit/push/READY publication, full current-head hosted
review/CI and separate ROOT serial merge authorization remain pending at this
local checkpoint. No hosted or merge success is claimed by these artifacts.

## Risks and handoff gates

- A missing reactive dependency could retain a stale map without a new cache
  collection reference. Tests must vary context and cache stamps independently.
- Existing grouping fixtures were adapted to valid active/cache context and
  consistent row IDs without weakening their positive controls.
- Scope-safe emptiness can persist until a separate existing path publishes
  current data. Do not expand this work order into refresh scheduling.
- Dependencies were absent at design time. ROOT's later release and exclusive
  local-heavy lease preceded the one conditional frozen install.

The design handoff and ROOT review preceded the later same-primary
implementation. Publication uses normal active hooks, a new
Conventional Commit and a READY PR only after task checks. Preserve the live
task plan's exact-head CI/reviewer, exclusive-heavy RETURN, single original
`scripts/pr-await` collector, independent verification and separate ROOT MERGE
grant. Design completion does not complete the task or authorize merge.
