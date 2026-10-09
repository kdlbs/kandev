---
id: "01-refresh-maintenance-roadmap"
title: "Reconcile the architecture-maintenance roadmap"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001
acceptance_criteria:
  - AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.4
system_design:
  - ../../specs/architecture-lint/system-design/root-state-slice-typing.md
---

# Task 01: Reconcile the architecture-maintenance roadmap

## Summary

Update the architecture-maintenance records to reflect verified work merged by
2026-10-09. Keep historical inventory counts attached to their original date
and source commit, and show the fresh current-main measurement separately.

## In scope

- Close the completed “prove the second resource” milestone with verified
  database-statistics, typed Azure DevOps, and first Office alias-retirement
  delivery evidence.
- Record the four Query-owned System snapshots, selected official Query ESLint
  rules, the LINT-02 owner guard, the refreshed architecture overview, and the
  current Office alias count.
- Preserve remaining proposals/deferred candidates with honest reasons.
- Correct the outdated SystemInfo-only migration summary after checking the
  current provider and hook owners.
- The combined [PR #4375](https://github.com/kdlbs/kandev/pull/4375) is open.
  The roadmap records the Jira/Linear implementation as complete with delivery
  pending until merge evidence exists.
- Update only the five roadmap files named below.

## Out of scope

- Any source, test, configuration, or baseline change.
- Editing/deleting the July audit source files, inventing old CI results, or
  marking Jira/Linear merged before this PR merges.
- Creating an umbrella issue, scheduling automation, or selecting another
  migration.

## Acceptance

- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.4`: Completed outcomes link
  to verified merged PRs; historical counts retain their dated source; the
  fresh snapshot names current main; and no open Jira/Linear work is described
  as merged.
- The current System migration record shows Query ownership for About
  SystemInfo, database statistics, the backup list, and the disk-usage
  snapshot, while retaining per-resource boundaries and the System job stream.
- The current linter record names the four selected `@tanstack/query` rules and
  records LINT-02 as implemented by PR #4357 at merge commit
  `9ad5964ca0a6bdce1e32b462450f7ba78f9cf2b8`.
- The Office inventory records PR #4010 as the first alias-retirement
  increment, with 27 Office alias registrations remaining in the 2026-10-09
  main snapshot. Historical 31-alias and 33-entry counts remain dated to
  2026-09-27.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/architecture-maintenance
git status --short -- docs/architecture-maintenance
```

Merged evidence to verify and retain:

- [#4009](https://github.com/kdlbs/kandev/pull/4009), typed Azure DevOps slice,
  merge commit `acfba523ff363096e0793045ffc94e27479e8a76`.
- [#4010](https://github.com/kdlbs/kandev/pull/4010), first Office event-alias
  retirement, merge commit `5907a4619757254cbb645fad87b20fc3e8742489`.
- [#4011](https://github.com/kdlbs/kandev/pull/4011), refreshed architecture
  overview, merge commit `d2b66efd7e48fc518a9d62131d6d1392397b43db`.
- [#4012](https://github.com/kdlbs/kandev/pull/4012), selected official
  TanStack Query rules, merge commit `a1e2edadb9cd40a08d23a0f9b72665146ec3fea5`.
- [#4225](https://github.com/kdlbs/kandev/pull/4225), database-statistics
  Query migration, merge commit `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`.
- [#4271](https://github.com/kdlbs/kandev/pull/4271), backup-list Query
  migration, merge commit `d149627883ca74de0dde98c1900415729af44bbd`.
- [#4291](https://github.com/kdlbs/kandev/pull/4291), disk-usage Query
  migration, merge commit `122f52018c77b9fb3c5c93f58f52bc7ea9b76904`.
- [#4357](https://github.com/kdlbs/kandev/pull/4357), LINT-02 owner guard,
  merge commit `9ad5964ca0a6bdce1e32b462450f7ba78f9cf2b8`.

## Files likely touched

- `docs/architecture-maintenance/README.md`
- `docs/architecture-maintenance/server-state-migrations.md`
- `docs/architecture-maintenance/dependency-cleanup.md`
- `docs/architecture-maintenance/lint-roadmap.md`
- `docs/architecture-maintenance/historical-audit.md`

## Dependencies

None. This is the first work order.

## Risks

- The 2026-09-27 inventory counts are historical evidence and must not be
  silently replaced by the 2026-10-09 measurement.
- Creating the live Jira/Linear link depends on the combined PR being open.
- The July audit files are outside the owned paths and remain untouched.

## Parallelism

`sequential`

## Inputs

- The verified PR links and merge commits above.
- Current source ownership in the System hooks and `apps/web/AGENTS.md`.
- The existing architecture-maintenance records and architecture-lint
  requirement/design for this increment.

## Results

Reconciled the five roadmap records. The completed milestone now links merged
PRs #4009, #4010, #4011, #4012, #4225, #4271, #4291, and #4357 with their
verified merge commits. Historical counts remain attached to the 2026-09-27
snapshot. Current main `64f8830d0d7b1ea98da96d3373d68bc0386cf49e` is recorded
separately with 45 root-state-cast entries, 29 compatibility-ledger
registrations including 27 Office aliases, and four Query-owned System
snapshots. The System job stream remains identified as Zustand-owned.

Passed at Task 01 completion on main `64f8830d0d7b1ea98da96d3373d68bc0386cf49e`:

- `python3 scripts/list-docs.py validate` — 368 decisions and 1,495 specifications validated.
- `python3 scripts/lint-spec-files.py --all` — all specification files passed.
- `git diff --check -- docs/architecture-maintenance` — passed.
- `git status --short -- docs/architecture-maintenance` — listed only the five owned roadmap records.

Merged evidence commit ancestry was verified against current main with
`git merge-base --is-ancestor` for each of the eight listed PR merge commits.

After the disjoint main-only update, the unpublished branch was fast-forwarded
to `3fed5570cec533f468c25ed03c84967bdc972588`. The latest root-state-cast
count remains 45, and the compatibility ledger remains at 29 registrations
including 27 Office aliases. The roadmap snapshot links were refreshed to this
commit. Revalidation passed: the docs catalog reports 368 decisions and 1,498
specifications, all-spec lint passed, and `git diff --check` passed.

The Jira and Linear implementation is complete, with delivery pending under
[PR #4375](https://github.com/kdlbs/kandev/pull/4375).
