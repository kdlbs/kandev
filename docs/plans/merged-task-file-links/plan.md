---
created: 2026-10-26
status: implemented
requirements:
  - REQ-UI-EXTERNAL-VCS-FILE-LINKS-001
system_design:
  - ../../specs/ui/system-design/external-vcs-file-links.md
legacy_specs: []
---

# Implementation Plan: Merged Task File Links

## Overview

Repair external file actions after GitHub merges delete a source branch. One
sequential work order preserves source-branch matching while converting the
selected merged PR to its saved head revision. The existing UI requirement owns
this reusable cross-provider file-navigation contract.

ROOT read all four artifacts and sent a later explicit implementation release
on 2026-10-26 at 19:58, with the exclusive heavy lease for this task. The work
order was implemented in the same primary session. No agents, tasks, or sessions
were spawned. Hosted delivery and the separate merge gate remain external.

## Evidence and requirement conformance

At diagnosis, source inspection confirmed `publishedBranches` mapped each GitHub `TaskPR` to
`head_branch`, losing `state` and `head_sha`. `resolvePublishedBranch` then
returned that name unchanged, and the real component rendered it as its `href`.
Thus a merged PR still targeted its retired source branch. The requirement's
AC .4 lacked a terminal-lifecycle exception; AC .9 defines it, and the missing
paired design now documents selection and fallbacks.

ROOT supplied an independent qualification summary: PR4816 source branch
`feature/fix-live-attachment-t1l` absent, contents lookup at that branch 404,
saved head `6383c671c357e5f9cd4a9dc79ed9ac7d23f17c80` available; real rendered
`sm` and `touch` actions failed while open/base controls passed. This task has
not read, copied, or run ROOT's private fixture/evidence source. Independent
rendered RED/GREEN reproduced the same defect during implementation below.

## Scope

In scope: merged GitHub target selection from existing metadata; missing-SHA
fallback; explicit override precedence; repeated repository/branch ambiguity;
real hook-to-anchor regression evidence on compact and touch actions.

Out of scope: publishing, provider/fork provenance redesign, backend/schema/API
changes, other terminal provider behavior, deleted-file history redesign,
layout/copy changes, issues, and unrelated PRs. No new ADR is needed: this is a
local metadata-selection correction inside the existing navigation boundary.

## Technical approach

Change `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts` to keep
candidate branch identity until repository/active-branch selection completes,
then derive the GitHub revision from state/head SHA. Retain explicit
`publishedBranch` priority. Preserve all repository and ambiguity guards. Use
the existing URL helper's revision/path rules without changing its public API.
For duplicate source-branch rows, identical targets deduplicate; conflicting or
partly missing target evidence falls back safely rather than depending on order.

| Provider/context | Target | Evidence/fallback |
| --- | --- | --- |
| GitHub merged, saved SHA | Saved task head snapshot | New hook and real anchor regressions |
| GitHub merged, missing SHA | Existing base/path rules | Added/untracked omit; renamed previous path; hook tests |
| GitHub open/unmerged closed, including unknown fork provenance | Existing branch | Existing and focused control regressions |
| Explicit publishedBranch | Caller branch | New precedence control |
| GitLab including self-hosted / Azure DevOps | Existing branch | Existing hook/URL suites; no new lifecycle support |
| Wrong/ambiguous repository or conflicting candidate | Existing safe fallback or omission | Mixed-repository/repeated-branch tests |

## ASCII UI preview

UI-01: Existing task file/diff toolbar, merged GitHub PR. Shared composition:

```text
Before: [file name] [existing actions] [GitHub icon]
        href = /blob/<deleted-source-branch>/<file>
After:  [file name] [existing actions] [GitHub icon]
        href = /blob/<saved-head-sha>/<file>
```

AC .2/.9: the anchor stays in the same order and opens a new tab. On phones the
existing `MobileFileViewerPanel` inline touch action uses the same target and
keeps its existing 44px size. Content remains the current scroll owner. No
geometry, composition, or copy changes are planned; spacing above is illustrative.

## Tests

AC .4/.9: use `use-external-vcs-file-link.merged.test.tsx` with merged SHA, missing/
empty SHA, open-with-SHA and unmerged closed controls, and explicit override.
Use descriptive test names such as `uses the saved head revision for a merged
GitHub PR` and `keeps an explicit published branch for a merged GitHub PR`.

AC .5/.9: repeated-repository tests mix merged and open siblings with distinct
source branches and SHAs; select each active branch, reject sibling reuse,
wrong repository/legacy identity, and conflicting same-branch candidates.

AC .8/.9: parameterized merged tests cover modified, added, untracked, renamed
new path, deleted base path, and missing-SHA renamed old path/added omission.
Run existing `external-vcs-file-url.test.ts` and GitLab host tests unchanged.

AC .1/.2/.9: new `external-vcs-file-link.merged.test.tsx` renders real
`StateProvider` + real hook + `ExternalVcsFileLink`, without mocking link
resolution, for `sm` and `touch`. Assert actual href, label, new-tab attributes,
and unchanged action classes. Both merged cases must fail before the fix while
open and no-PR base controls pass. Use independently authored state fixtures.

## E2E tests

Use the explicit mobile-parity state/data-normalization exception: the shared
real-component regressions above exercise actual store-to-anchor output on both
action sizes without changing layout or touch interaction. Existing browser
specs `e2e/tests/review/external-vcs-file-link.spec.ts` and
`e2e/tests/task/mobile-external-vcs-file-link.spec.ts` retain baseline browser
navigation/phone geometry coverage. No new Playwright run or broad replay is
part of this repair; component output is the changed boundary.

## Work orders

- [x] [Task 01: Preserve merged GitHub file revisions](task-01-merged-revisions.md)

## Verification results

Design checkpoint: `python3 scripts/list-docs.py validate` passed (403 decisions,
1741 specifications); `python3 scripts/lint-spec-files.py --all` and
`git diff --check` passed. The repository's `validateCoverage` preflight accepted
the four artifacts with the prospective hook path, returning `covered` and zero
errors. This is cross-reference evidence, not evidence of an implemented diff.
The first preflight attempt found no `node` on PATH; rerunning with the existing
Node 24.21.0 absolute path succeeded. The work order records the required PATH.

Implementation results: independent rendered RED failed only the two merged
anchors while four open/base controls passed; hook RED had 16 expected failures
and 21 passing controls. Final focused GREEN passed six suites and 114 tests.
Affected canonical lint passed after the necessary test-file split. The default
4 GiB typecheck exited 134 (V8 heap exhaustion); the same bounded retry with
`NODE_OPTIONS=--max-old-space-size=6144`, GNU 6-minute timeout and 10-second
kill grace joined with exit 0. No further resource retry was launched.

Frozen installation, documentation catalog, 36 spec-linter tests, all-spec lint,
63 public-doc validator tests, 54-page public-doc validation, whitespace diff,
and actual staged-file PR-documentation coverage all passed. The work order
records exact commands. Local implementation is complete; commit/hosted evidence
is tracked in the task plan, without claiming remote gates are complete.

## Implementation adjustments

The first focused GREEN passed 114 tests, then canonical lint rejected the
expanded existing hook suite for file/function size and repeated literals. Its
unchanged fixture builders were moved into a shared test-only helper, and new
merged cases went into a separate suite, preserving the tests and production
boundary. Both suites/helper are included in affected checks.
The public sessions/review guide already documents source-branch precedence;
only that paragraph was updated for merged head snapshots and fallback.

## Risks

- Replacing branch names before matching would lose repeated-worktree identity.
- Legacy missing-SHA rows cannot establish a published snapshot; base fallback
  remains best effort, and added/untracked actions must stay suppressed.
- Fork metadata remains incomplete; preserve existing behavior and do not infer
  alternate repositories or PR refs.
- A saved head is the task snapshot, not a claim about the final merged tree.
