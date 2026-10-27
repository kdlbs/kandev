---
id: "01-merged-revisions"
title: "Preserve merged GitHub file revisions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EXTERNAL-VCS-FILE-LINKS-001
acceptance_criteria:
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.1
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.2
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.4
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.5
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.8
  - AC-UI-EXTERNAL-VCS-FILE-LINKS-001.9
system_design:
  - ../../specs/ui/system-design/external-vcs-file-links.md
---

# Task 01: Preserve Merged GitHub File Revisions

## Summary

Make inferred merged GitHub file actions use the saved head snapshot while
preserving source-branch identity for selection. Author an independent rendered
regression that fails on the current desktop and touch anchors, then implement
the smallest hook correction.

## In scope

- Source identity versus target revision; saved SHA and missing-SHA fallback.
- Existing explicit override, repository matching, file status, and ambiguity
  behavior, with mixed merged/open repeated-repository coverage.
- Actual store/hook/component output in both existing action sizes.

## Out of scope

Backend, schemas, publishing, new provider lifecycle support, fork provenance,
deleted-file history redesign, UI composition/copy, broad testing, and ROOT's
private or historical fixtures. Do not revert other contributors' work.

## Acceptance

1. Independent real-hook/component merged `sm` and `touch` href regressions fail
   for the deleted-branch target before the fix, while open/base controls pass;
   all pass afterward with saved SHA URLs and unchanged anchor semantics.
2. State normalization selects source identity before target revision, preserves
   explicit override and repeated-repository guards, and never picks conflicting
   candidates by order. Hook regressions cover mixed live/merged and missing SHA.
3. Status/path and supported-provider controls pass; exact checks below are
   recorded, both delivery statuses are updated, and no unrelated code changes
   or UI geometry changes are introduced.

## ASCII UI preview

UI-01 from [the full plan](plan.md#ascii-ui-preview), AC .2/.9:

```text
[file name] [existing actions] [GitHub icon]
Before href: /blob/<deleted-source-branch>/<file>
After href:  /blob/<saved-head-sha>/<file>
```

Desktop `sm` and phone `touch` retain their existing composition/sizes, new-tab
behavior, accessible name, and scroll ownership. Test the real shared action.
The mobile-parity normalization exception applies because no responsive or
interaction presentation changes. Compare rendered semantics/classes to this
preview; no new geometry evidence is claimed by a DOM component test.

## Verification

Only after ROOT's later explicit implementation interrupt and serialized heavy
lease: read `/tdd`, install once in this fresh worktree, and run RED before the
production edit. Commands run from the repository root:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/editors/external-vcs-file-link.merged.test.tsx)
```

Record RED failures for both merged action sizes and passing open/base controls.
Implement, then run one focused GREEN selection (which covers all changed suites):

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.merged.test.tsx hooks/domains/workspace/use-external-vcs-file-link.gitlab-host.test.tsx components/editors/external-vcs-file-link.test.tsx components/editors/external-vcs-file-link.merged.test.tsx lib/utils/external-vcs-file-url.test.ts)
(cd apps/web && pnpm run lint -- hooks/domains/workspace/use-external-vcs-file-link.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx hooks/domains/workspace/use-external-vcs-file-link.merged.test.tsx hooks/domains/workspace/use-external-vcs-file-link.test-helpers.tsx components/editors/external-vcs-file-link.merged.test.tsx)
(cd apps/web && timeout --kill-after=10s 6m env NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

No full suite, backend tests, or new Playwright work is required. If additional
files change for a concrete correctness reason, include their lint and affected
test suite explicitly. Use configured worker budgets. Any `scripts/run-quiet`
wrapper needs its own task-owned `KANDEV_RUN_QUIET_DIR` per invocation. Retain
and join every native command handle; return the heavy lease concisely after
all owned work exits, then proceed to authorized normal hosted delivery.

## Files likely touched

- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx`
- `apps/web/components/editors/external-vcs-file-link.merged.test.tsx` (new)
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.merged.test.tsx` (new)
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test-helpers.tsx` (shared existing fixture extraction required by lint limits)
- `docs/public/sessions-and-review.md` (existing external-file revision paragraph)
- This plan/work order and the paired requirement/system design for final results.

Read-only controls: `external-vcs-file-link.tsx`, its existing test, the URL
utility/test, GitLab-host suite, and current mobile toolbar. Avoid changing the
utility or component unless reproduction proves a correction there is necessary.

## Dependencies

None. Execution requires ROOT's full four-artifact review, later separate explicit
implementation interrupt, and serialized heavy-lease grant. This work order is
not implementation or delegation authorization by itself.

## Risks

See the [plan risks](plan.md#risks). A duplicate same-branch candidate with
different or missing target evidence must fall back, not choose a sibling SHA.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/external-vcs-file-links.md), especially AC .4/.5/.8/.9.
- [System design](../../specs/ui/system-design/external-vcs-file-links.md).
- Current hook tests provide independent state fixture patterns; do not use
  ROOT private evidence or historic deferred/canceled-task fixtures.

## Results

Completed locally after ROOT's separate 2026-10-26 19:58 release.

- Frozen workspace installation passed once.
- Rendered RED command above: exit 1, exactly two merged-anchor href failures
  (`sm` and `touch`), with all four open/base controls passing.
- Hook RED before production edits: exit 1, 16 expected failures and 21 passing
  controls. New merged cases were subsequently moved to their own suite and
  existing fixture builders extracted to satisfy canonical lint limits.
- Final six-suite GREEN command above: exit 0, 114 tests passed.
- Affected canonical lint command above: exit 0, zero warnings.
- Default `pnpm run typecheck`: exit 134, V8 default 4 GiB heap exhaustion.
  The single bounded 6144 MiB retry command above joined exit 0. No replacement
  or further memory increase was launched.
- `python3 scripts/list-docs.py validate`: exit 0, 403 decisions/1741 specs.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `node --test scripts/validate-public-docs.test.mjs`: exit 0, 63 tests.
- `node scripts/validate-public-docs.mjs`: exit 0, 54 pages.
- `git diff --check` and actual staged-path `validateCoverage` from
  `.github/scripts/pr-docs.cjs`: passed; coverage returned `covered`, zero errors.

Rendered UI-01 semantics and unchanged compact/touch action classes matched the
preview; no browser geometry claim is made. The mobile normalization exception
applies. Public docs updated: one paragraph in the existing sessions/review
how-to guide. ROOT private evidence and protected/foreign resources were not
accessed. Normal hook, commit, and hosted delivery receipts remain in the task
plan; remote CI/review and merge gates are pending independently of local done.
