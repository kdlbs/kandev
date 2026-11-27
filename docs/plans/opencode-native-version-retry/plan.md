---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Implementation Plan: Retry the native OpenCode version check

## Overview

A native OpenCode launch runs `opencode --version` to pick compatible ACP
arguments. The check had one 3-second attempt. `opencode --version` boots a
JavaScript runtime, and with many launches at once a healthy run took more than
3 seconds on Windows. The deadline kill surfaced only as
`read native OpenCode version: exit status 1`, the output was dropped, and the
launch failed although the binary was fine.

One work order bounds each run at 10 seconds, retries transient failures,
reuses a recent detection of the same executable, and makes the final
diagnostic explain itself. See
[task 01](task-01-retry-native-version-check.md).

## Scope

### In scope

- `DetectOpenCodeNativeRuntime` in `apps/backend/internal/agent/agents`.
- Deterministic unit tests with an injected version runner, clock, and sleep.

### Out of scope

- Managed (npm) runtime detection and selection.
- The bootstrap precedence rules and the stored runtime record.
- Changing which majors are supported.

## Verification

- `go test ./internal/agent/agents/` (new detector tests).
- `golangci-lint run ./internal/agent/agents/...`.
- `make -C apps/backend build`.
