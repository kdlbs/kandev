---
id: "04-verification"
title: "Portable adoption verification"
status: done
wave: 4
depends_on: [3]
plan: "plan.md"
requirements:
  - REQ-TASKS-PORTABLE-ADOPTION-001
  - REQ-TASKS-PORTABLE-ADOPTION-002
  - REQ-TASKS-PORTABLE-ADOPTION-003
system_design:
  - ../../specs/tasks/system-design/portable-workspace-adoption.md
acceptance_criteria:
  - AC-TASKS-PORTABLE-ADOPTION-001.1
  - AC-TASKS-PORTABLE-ADOPTION-001.2
  - AC-TASKS-PORTABLE-ADOPTION-001.3
  - AC-TASKS-PORTABLE-ADOPTION-002.1
  - AC-TASKS-PORTABLE-ADOPTION-002.2
  - AC-TASKS-PORTABLE-ADOPTION-002.3
  - AC-TASKS-PORTABLE-ADOPTION-002.4
  - AC-TASKS-PORTABLE-ADOPTION-003.1
  - AC-TASKS-PORTABLE-ADOPTION-003.2
  - AC-TASKS-PORTABLE-ADOPTION-003.3
  - AC-TASKS-PORTABLE-ADOPTION-003.4
---

# Task 04: verification

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: docs/public/git-operations.md; task-owned regression tests.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: Task 03.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted race checks and normal commit hooks passed. Independent Review and
QA passed source candidate 7cb27830c637ba2969c99271b9e982f3a29093ea; see the plan
for exact commands, session IDs, report paths and limitations. MAIN resolved both
Review findings with permanent regression coverage. Final documentation-only
candidate verification is recorded in Kandev plan metadata to avoid a self-SHA
documentation loop. No PR, deployment or local-overlay retirement is claimed.

## Maintainer review remediation

Original hosted full-backend evidence failed the previous-stable upgrade inventory
for the new receipt table. The manifest now lists that table without modifying
the immutable stable SQL fixtures. Initial main integration preserves both the
task-creation authorization and checkout-adoption readiness entries.

An isolated exact-source selection proof reproduced refusal of an ordinary
generated worktree whose attachment keeps an empty checkout selector. Selection
now shares the launch branch-identity planner and receipt validation retains the
attachment selector/base branch. Permanent handler coverage exercises that path
and SQL CAS covers base-branch mutation.

Hosted verification is pending for the full packages and the two runtime/auth
cases in `apps/web/e2e/tests/session/workspace-adoption.spec.ts`. Those cases use
a real native mock process, production HTTP/auth middleware, graceful backend
stop/restart, operator adoption/replay, persisted logical identities, authenticated
WebSocket resume and file reads from the independent checkout. They introduce no
UI surface or viewport-dependent behavior. No full-runtime acceptance is claimed
until the actual hosted cases pass.

Security remediation requires a verified, non-synthetic administrator at both
the composition and service boundaries. Auth-disabled no-header requests against
an idle real task are refused. The source's retired administration parent is
pinned without following links and revalidated before commit/reuse. Both source
and candidate now receive the same metadata-layout and ref/object-storage checks;
source HEAD/configuration/object symlinks, alternates and promisor packs are refused.
Permanent HTTP regressions cover preview, apply rollback, receipt/replay and reuse,
including replacement of a retained administration-parent handle. Bounded source
checks reproduced these defects and passed after correction; real package and
runtime acceptance still requires the hosted command results.

Publication also captures attachment metadata in its CAS digest. A PR-number
mutation can change the shared branch planner's duplicate identity slug, so it
must invalidate a pending preview even when the branch strings remain equal.
SQLite and PostgreSQL selection-CAS regressions include this mutation. This is a
publication snapshot check; receipt reuse continues to validate its retained
ownership, selected branch and filesystem authority.

Publication and receipt checks also retain recovery's empty-or-active legacy
status semantics. SQLite/PostgreSQL regressions prove refusal of a candidate
already held by an active legacy slot and successful adoption of a selected
legacy slot without changing its raw status. The narrow source-query predicate
reproduced the omitted legacy binding and passed after correction; full repository
and PostgreSQL acceptance remains hosted.

Full hosted handler events exposed nil-versus-empty metadata normalization in the
new CAS comparison; both sides now share an empty-metadata digest. Existing real
HTTP positive cases retain their 200 assertions, and SQLite/PostgreSQL coverage
checks equivalent null metadata. PostgreSQL 16 cutover leaves also exposed the
receipt table's environment foreign key: synthetic legacy rewinds remove the
future table, while production cutover preserves any existing receipt data and
rebinds its constraint. A permanent hybrid PostgreSQL regression retains exact
receipt JSON/environment identity and verifies the surviving foreign key/cascade.
