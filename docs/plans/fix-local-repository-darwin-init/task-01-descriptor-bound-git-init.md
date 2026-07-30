---
id: "01-descriptor-bound-git-init"
title: "Descriptor-bound Git initialization"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/create-local-repository/spec.md"
---

# Task 01: Descriptor-Bound Git Initialization

## Acceptance

- Linux and macOS initialize the exact verified directory inherited as fd 3; replacing its pathname
  cannot redirect Git into the replacement.
- Successful initialization still creates only `.git`, uses unborn branch `main`, and preserves the
  service's identity check, exclusive publication, rollback, and error reporting.
- Windows retains its existing pathname-compatible behavior, while comments and PR scope no longer
  claim unsupported BSD product behavior.

## Verification

```bash
cd apps/backend && go test ./internal/task/gitinit ./internal/task/service ./cmd/kandev \
  -run 'GitInit|InitializeLocalRepository|Dispatch' -count=1
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
- `apps/backend/cmd/kandev/main.go`
- `apps/backend/cmd/kandev/main_test.go`
- `apps/backend/internal/task/service/local_repository_initialization.go`
- `apps/backend/internal/task/service/local_repository_initialization_test.go`
- `docs/specs/create-local-repository/spec.md`
- `docs/plans/fix-local-repository-darwin-init/plan.md`
- `docs/plans/fix-local-repository-darwin-init/task-01-descriptor-bound-git-init.md`

## Dependencies

None.

## Parallelism

Sequential. The helper and service changes share one subprocess contract and one regression test.

## Inputs

- Spec: exact-directory identity requirement, staging replacement failure mode, and regression
  scenario.
- Existing hidden `__backend` dispatch in `apps/backend/cmd/kandev/main.go`.
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
- GREEN: focused helper/service/dispatch coverage passed, 28 tests in 3 packages.
- Full affected packages passed, 776 tests in 3 packages.
- Race-focused helper and service coverage passed, 22 tests in 2 packages.
- Darwin arm64 cross-compilation passed for the service test binary and `cmd/kandev`.
- `make -C apps/backend lint` passed with 0 issues.
