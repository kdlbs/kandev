---
id: "01-scan-aliased-os-reads"
title: "Scan aliased os environment reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.4
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.1
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
  - ../../specs/integrations/system-design/github-authentication-01.md
---

# Task 01: Scan aliased os environment reads

## Summary

Close the classification gap that let an aliased or dot-imported
`os.Getenv` / `os.LookupEnv` call bypass the shared hermetic-environment
guard, and adopt that guard for the GitHub suite instead of its
package-local copy.

## In scope

- `apps/backend/internal/testutil/envscan.go`
- `apps/backend/internal/testutil/envscan_test.go`
- `apps/backend/internal/github/env_hermetic_test.go`

## Out of scope

Production credential resolution, new environment fallbacks, bulk unnamed
reads such as `os.Environ`, non-`os` accessors such as `syscall.Getenv`, and
public user documentation.

## Acceptance

- An aliased `os` import is classified and reports an uncovered environment
  variable; the regression was red before the scanner change and green after.
- Dot-imported bare `Getenv` / `LookupEnv` calls and shadowed local `os`
  declarations are classified by their actual binding, not by spelling.
- Existing plain-import scanning, extra readers, and non-`os` selectors keep
  their behavior; the GitHub, GitLab, and process hermetic suites pass
  against the shared scanner.
- The GitLab `GITLAB_TOKEN` environment fallback and the GitHub
  workspace-source selection stay testable without ambient shell credentials
  sabotaging the scrubbed fixtures.

## Verification

Run from `apps/backend`:

```bash
GOMODCACHE=<workspace module cache> GOCACHE=<workspace build cache> \
  go test -count=1 ./internal/testutil ./internal/github/... ./internal/gitlab/...
gofmt -l internal/testutil/envscan.go internal/testutil/envscan_test.go internal/github/env_hermetic_test.go
```

## Files likely touched

- `apps/backend/internal/testutil/envscan.go`
- `apps/backend/internal/testutil/envscan_test.go`
- `apps/backend/internal/github/env_hermetic_test.go`
- This plan and work order.

## Dependencies

None. The GitLab and process scrubs already used the shared scanner.

## Risks

The `go/types` pass resolves bindings per file and ignores type errors
elsewhere; keeping that tolerance documented avoids a future "fix" that
makes the guard fail on unrelated broken code.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md)
- [GitLab requirements](../../specs/integrations/requirements/gitlab-integration.md), criterion `AC-INTEGRATIONS-GITLAB-INTEGRATION-001.4`
- [GitLab system design](../../specs/integrations/system-design/gitlab-integration-02.md)
- [GitHub authentication requirements](../../specs/integrations/requirements/github-authentication.md), criterion `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.1`
- [GitHub authentication system design](../../specs/integrations/system-design/github-authentication-01.md)

## Results

The scanner derives the file-local import name bound to package path `os`
through `go/types`, so alias and dot-import forms are classified exactly like
plain `os.Getenv` / `os.LookupEnv` calls. `internal/github` now reuses the
shared `testutil.AssertEnvReadsCovered` instead of its package-local copy.

Verification on 2026-09-09 (focused suites) recorded in the PR:

- `go test -count=1 ./internal/testutil ./internal/github/... ./internal/gitlab/...`: passed.
- `gofmt -l`: clean on the changed files.

The aliased-import regression was red before the scanner change and green
afterward. Existing standard `os` calls, extra readers, and non-`os`
selectors retain behavior.
