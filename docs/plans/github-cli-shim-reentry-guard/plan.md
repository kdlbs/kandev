---
created: 2026-09-25
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-03.md
legacy_specs: []
---

# Implementation Plan: GitHub CLI shim re-entry guard

## Overview

`agentctl`'s `gh` shim removed only `$KANDEV_GITHUB_CLI_SHIM_DIR` from `PATH`
before looking up the real `gh`. When that variable was empty or named a
different shim directory (a stale directory left on `PATH` by an earlier
`agentctl`), the lookup returned the shim itself and each invocation launched
another one. On one host this produced about 1,500 `gh pr view --json url`
processes, filled RAM and swap, and took Kandev down.

## Scope

### In scope

- Skip every `gh` inside a `kandev-github-cli-*` directory, and the running
  `agentctl` binary itself, during the real-CLI lookup.
- Pass a shim depth to the launched CLI and refuse to run at the bound, so a
  shim that still resolves to a shim fails after a handful of levels while a
  `gh` extension calling `gh` keeps working.
- Record the guard in the managed-routing design and scenarios.

### Out of scope

- Cleaning stale shim directories out of an inherited `PATH`.
- Any change to lease redemption or the shim's `gh` configuration isolation.

## Technical approach

Both guards live in `apps/backend/cmd/agentctl`. `main.go` builds the
shim-skipping lookup from `os.Executable`. `github_cli_shim.go` parses the
inherited depth before any other work and sets depth+1 on the child it
launches. Identity alone was not enough: a stale shim directory can link to
an older `agentctl`, and a Windows shim is a copy, so the directory prefix
`installGitHubCLIShim` uses is the durable signal. A blanket re-entry refusal
broke nested calls through `BASH_ENV`, hence the bound instead.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.14` | `TestLookPathSkippingShimsIgnoresLinksToSelf`, `TestLookPathSkippingShimsIgnoresShimDirectories`, `TestGitHubCLIShimIncrementsChildDepth`, `TestGitHubCLIShimRefusesAtDepthBound`, and `TestGitHubCLIShimRejectsMalformedDepth` in `github_cli_shim_test.go`; existing `TestGitHubCLIShim*` cases keep passing for a real CLI. |

## Work orders

- [x] [Task 01: Guard the gh shim against re-entry](task-01-guard-shim-reentry.md)

## Verification results

- `cd apps/backend && go test ./cmd/agentctl -count=1` passes.
- `gofmt -l` and `go vet` are clean for `./cmd/agentctl`.
