---
id: "01-retry-native-version-check"
title: "Retry and reuse the native OpenCode version check"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.11
  - AC-AGENTS-OPENCODE-V2-001.12
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01: Retry and reuse the native OpenCode version check

## Summary

Make the native OpenCode version check tolerate a slow or transiently failing
run without weakening the compatibility gate.

## Changes

- Move detection into `opencode_native_detect.go` behind a small detector with
  seams for the executable lookup, the version run, the retry wait, and the
  clock.
- Bound each run at 10 seconds; retry a failed, timed-out, or unreadable run
  twice (300 ms, then 1 s).
- Remember a successful detection per executable path for 10 minutes and use
  it when a later check still fails after its retries.
- Treat a missing executable (absent) and an unsupported major (compatibility
  error, cache cleared) as definite answers that are not retried.
- Mark a deadline kill in the error and quote a bounded, home-redacted,
  single-line excerpt of the output.

## Acceptance

1. A run that fails, then prints no version, then prints `1.18.34` yields the
   v1 runtime after waits of 300 ms and 1 s.
2. After the last retry the error names the timeout and a bounded excerpt
   without the home directory.
3. A recent detection of the same path is returned when every run fails; it
   expires after 10 minutes and is not shared across paths.
4. An unsupported major is returned after one run and is not served from the
   cache afterwards; a missing executable is still reported as absent.
5. A caller that cancels its context stops the retries.

## Verification

- `go test ./internal/agent/agents/ -run 'TestOpenCodeNativeDetection|TestDetectOpenCodeNativeRuntime'`
- `golangci-lint run ./internal/agent/agents/...`
- `make -C apps/backend build`
