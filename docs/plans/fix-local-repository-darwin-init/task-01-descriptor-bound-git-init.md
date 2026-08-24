---
id: "01-descriptor-bound-git-init"
title: "Descriptor-bound Git initialization"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/create-local-repository.md"
---

# Task 01: Descriptor-Bound Git Initialization

## Acceptance

- Linux and macOS initialize the exact verified directory inherited as fd 3; replacing its pathname
  cannot redirect Git into the replacement.
- Successful initialization still creates only `.git`, uses unborn branch `main`, and preserves the
  service's identity check, exclusive publication, rollback, and error reporting.
- Windows retains its existing pathname-compatible behavior. Other Unix/BSD compatibility builds
  remain outside the supported product scope until they provide the same ownership and
  exclusive-publication guarantees.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
make -C apps/backend build

# Direct Go commands are approved exceptions: the backend Makefile has no target
# for package/test filters, the race detector, or CGO-free Darwin test binaries.
cd apps/backend && go test ./internal/task/gitinit ./internal/task/service \
  ./internal/task/handlers -run 'GitInit|InitializeLocalRepository' -count=1
cd apps/backend && go test -race ./internal/task/gitinit ./internal/task/service \
  -run 'GitInit|InitializeLocalRepository' -count=1
cd apps/backend && tmpdir=$(mktemp -d) && \
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c \
    -o "$tmpdir/local-repository-service.test" ./internal/task/service && \
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build \
    -o "$tmpdir/kandev" ./cmd/kandev
```

## Files Likely Touched

- `apps/backend/internal/task/gitinit/*.go`
- `apps/backend/internal/task/gitinit/*_test.go`
- `apps/backend/internal/task/service/local_repository_initialization.go`
- `apps/backend/internal/task/service/local_repository_initialization_test.go`
- `apps/backend/internal/task/handlers/repository_handlers_test.go`
- `docs/specs/workspaces/requirements/create-local-repository.md`
- `docs/plans/fix-local-repository-darwin-init/plan.md`
- `docs/plans/fix-local-repository-darwin-init/task-01-descriptor-bound-git-init.md`

## Dependencies

None.

## Parallelism

Sequential. The helper and service changes share one subprocess contract and one regression test.

## Inputs

- Spec: exact-directory identity requirement, staging replacement failure mode, and regression
  scenario.
- Go package initialization, used only by a marked helper subprocess before application/test entry.
- Existing `ExtraFiles` and identity checks in
  `apps/backend/internal/task/service/local_repository_initialization.go`.
- Explicit local repository trust boundary in
  `docs/decisions/2026-07-20-explicit-local-repository-trust.md`.

## Output Contract

Report the red test evidence, helper boundary, files changed, exact command results, residual cleanup
risk, commit and push receipt, and updated task/plan status.

## Verification Results

- RED: `go test ./internal/task/gitinit -run TestCommandContextRejectsMissingGit -count=1 -v`
  failed before parent-side Git resolution because command construction incorrectly succeeded.
- RED (CI remediation): the handler integration test timed out because the helper recursively
  launched a test binary that had no package-specific helper dispatch; reproduced locally with a
  15-second timeout.
- GREEN: focused helper/service/handler/CLI coverage passed, 35 tests in 4 packages, after moving
  the marked subprocess trampoline into the private package.
- Race-focused helper/service/handler coverage passed, 30 tests in 3 packages.
- The CI-shaped full race-and-coverage run completed every actual test package successfully,
  including handlers in 12.884 seconds. Its local command exit remained non-zero only because this
  environment's Go installation lacks `go tool covdata` for packages with no tests.
- Darwin arm64 cross-compilation passed for service and handler test binaries and `cmd/kandev`.
- `make -C apps/backend test` passed across the full backend.
- `make -C apps/backend build` passed for the unified binary and bundled helper binaries.
- `make -C apps/backend lint` passed with 0 issues after CI remediation.
- Full affected packages passed, 776 tests in 3 packages.
- Race-focused helper and service coverage passed, 22 tests in 2 packages.
- Darwin arm64 cross-compilation passed for the service test binary and `cmd/kandev`.
- `make -C apps/backend lint` passed with 0 issues.

## Delivery Record

- **Helper boundary:** Linux/macOS inherit the verified directory as fd 3; a subprocess carrying the
  private argument and environment marker calls `fchdir(3)` and execs Git before normal entry.
- **Changed files:** `internal/task/gitinit`, local-repository service/tests, handler integration
  coverage, the feature spec, and this plan/task record.
- **Residual cleanup risk:** if another process renames the request-owned inode, its new pathname may
  be unknowable; Kandev still leaves the replacement untouched and persists no repository row.
- **Initial delivery:** commit `46258170913b49ad40749ea4538134b6e4c393cf` was pushed to
  `nova28:fix/local-repo-init-darwin` with active pre-commit and commit-message hooks and no bypass.
- **Status:** task and plan completed; CI follow-up replaces package-specific dispatch after the
  handler integration test exposed recursive test-binary startup.
