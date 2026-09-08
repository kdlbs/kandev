---
id: "03-resolve-resume-repositories-once"
title: "Resolve resume repositories once"
status: done
wave: 2
depends_on: ["02-harden-origin-reconciliation"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Resolve Resume Repositories Once

## Acceptance

- Resume resolves the attached repository set once and reuses it for primary configuration,
  multi-repository configuration, and GitHub credential routing.
- Each attached managed repository reaches preparation/origin reconciliation at most once per
  resume.
- Persisted session primary selection, old-session fallback, repository ordering, branch metadata,
  repo-less tasks, and inherited task environments retain current behavior.
- A legitimate persisted primary absent from current attachments is resolved at most once through
  an explicit exceptional path rather than triggering another full-set resolution.
- Initial launch behavior and user-managed local checkout behavior remain unchanged.

## TDD Sequence

1. Add an instrumented multi-repository resume test that counts preparation calls and record the
   current repeated-primary RED result.
2. Add cases for persisted primary selection, old-session fallback, repo-less resume, and a
   persisted primary absent from the attachment set.
3. Refactor `buildResumeRequest` to resolve once and pass the result into primary, multi-repository,
   and credential consumers.
4. Retain existing transport transition, default-branch, task-directory, and environment tests.
5. Run the focused and package commands below and record GREEN.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'Test.*(Resume.*Repositor|BuildResumeRequest|RepoLocalPath.*Origin)' -count=1
cd apps/backend && go test ./internal/orchestrator/executor -count=1
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_test.go`
- `apps/backend/internal/orchestrator/executor/executor_multi_repo_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`
- a focused resume repository-resolution test file if clearer

## Dependencies

Task 02 establishes the no-op and error contract used by each preparation call.

## Parallelism

Sequential after Task 02 because both define the origin-reconciliation call boundary observed by
resume tests.

## Recorded Results

- RED: the multi-repository resume regression observed 5 origin-reconciliation calls for 2
  attached repositories because primary preparation, multi-repository configuration, and the outer
  credential-resolution pass each materialized the set.
- GREEN: the regression now observes 2 calls, one per attached repository; the full executor package
  passed 326 tests.
- The resume path now shares the resolved repository set across primary request fields, multi-repo
  specs, and GitHub credential routing. A persisted primary absent from current attachments keeps a
  single explicit fallback lookup.

## Output Contract

Report the RED call counts, final resolution flow, preserved fallback behavior, exact GREEN results,
and any remaining exceptional additional lookup. Update task and plan status.
