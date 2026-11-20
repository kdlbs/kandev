---
status: done
created: 2026-11-09
requirements:
  - REQ-OFFICE-PROVIDER-PROBE-001
system_design:
  - ../../specs/office/system-design/provider-health-probe.md
---

# Office provider probe npm prefix

## Overview

The provider health probe ran npm-based ACP adapters with the unresolved managed
npm prefix marker, so a retry failed with ENOENT and parked the provider's runs.
Resolve the marker in the probe the same way every launch path does.

## Technical approach

Call the existing `managednpm.PrepareNPMProjectPrefix` on a copy of the resolved
argv before spawning. No new prefix logic.

## Delivery order

1. [Task 01: Resolve the npm prefix in the probe](task-01-resolve-probe-npm-prefix.md)

## Verification strategy

- `go test -trimpath ./internal/agent/runtime/routingerr`
- `bash scripts/lint-go-changed` on the changed files.
