---
id: "01-implementation"
title: "fix(task): retain failed environments before reserving replacements"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.8
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# fix(task): retain failed environments before reserving replacements

## Outcome and scope

Retain failed physical environments and historical bindings while atomically reserving a fresh materializer/root for the requesting terminal session; environment-scoped worktree reuse.

Out of scope: Deletion or resetting of old files, broad discovery, live-consumer eviction, and operating data changes.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4713 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/tasks/system-design/task-launch-failure-recovery.md)
- [Plan](plan.md)

## Verification and results

Validation:
- Existing focused store/executor tests and temporary real-SQLite/ResumeSession checks passed, including binding-before-launch, same-reservation retry, retained inventory, ownership/consumer refusals, and preflight/launch failure release.
- SQL guard, documentation checks and all normal commit hooks passed.
- Required race conformance hit only an existing Unix-path storage fixture on Windows. Its failed scenarios and the real previous-stable upgrade passed with a temporary two-path Windows fixture overlay; assertions and product code were unchanged.
- Live PostgreSQL and full physical provider/worktree E2E were not run. No permanent tests, CI jobs or retirement endpoint were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The follow-up passed real SQLite binding/election/retention checks with -race (9.427s), actual Git preparation/cache checks with -race (16.539s), existing selected reuse controls (17.158s), and SQL guard. The additional native SQLite-to-cold-manager legacy-input control passed with -race (1.976s), reusing the same persisted checkout and preserving its marker file. Live PostgreSQL and live provider E2E remain unexecuted. Normal commit hooks must pass before publication.
