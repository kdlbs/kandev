---
id: "02-wrapper-preparation"
title: "Shared wrapper preparation"
status: done
wave: 2
depends_on:
  - "01-complete-design"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-004
acceptance_criteria:
  - AC-AGENTS-WRAPPERS-001.1
  - AC-AGENTS-WRAPPERS-002.1
  - AC-AGENTS-WRAPPERS-002.2
  - AC-AGENTS-WRAPPERS-002.3
  - AC-AGENTS-WRAPPERS-002.4
  - AC-AGENTS-WRAPPERS-004.3
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
---

# Task 02: Shared wrapper preparation

## Summary

Create the shared wrapper template and execution-host preparation boundary. Keep existing callers unchanged until their integration work orders.

## In scope

- Add the proposed `launchwrapper.Spec`, `Context`, and `Prepare` contracts from the design.
- Validate whole-token references and triple-brace escapes through the existing profile tokenizer and save/preview validation.
- Extract managed npm path preparation without changing private permissions, isolation, or symlink rejection.
- Resolve executable names with the final child PATH. Resolve explicit relative paths against the actual working directory.
- Keep previews non-executing. Ensure wrapper arguments cannot masquerade as trusted npm markers.

## Out of scope

No transport activation, process-manager integration, plugin API, or rendered UI change.

## Acceptance

1. A generated unrelated wrapper executable receives exact ordered arguments, including empty and metacharacter-containing values.
2. Runtime references resolve once on the execution host, and the npm reference matches the trusted inner command path.
3. Invalid references and executables fail before spawn. Literal and empty prefixes preserve existing behavior.

## Verification

Run from the repository root. Implement new behavior with failing tests first.
New test and harness paths below are delivery targets, not files already available in this design package.

```bash
(cd apps/backend && go test ./internal/common/launchwrapper ./internal/common/managednpm ./internal/agent/managedruntime ./internal/agent/settings/cliflags ./internal/agent/settings/controller)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from this work order when Go files change.
Record the expanded command and result. Do not replace targeted checks with an unrecorded broad suite.

## Files likely touched

- `apps/backend/internal/common/launchwrapper/` (shared preparation and tests).
- `apps/backend/internal/common/managednpm/` (shared npm project preparation and tests).
- `apps/backend/internal/agent/managedruntime/npm_project.go` and adjacent tests.
- `apps/backend/internal/agent/settings/cliflags/tokenise.go` and adjacent tests.
- `apps/backend/internal/agent/settings/controller/` prefix validation and preview tests.

## Dependencies

[Task 01](task-01-complete-design.md). Read its recorded results before implementation.

## Risks

A reference namespace changes formerly literal tokens. Test and document the explicit escape. Do not prepare paths during preview.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/generic-launch-wrappers.md).
- [System design](../../specs/agents/system-design/generic-launch-wrappers.md), especially the sections that match this work order.
- [Plan](plan.md), compatibility matrix and test mapping.
- [Historical reproduction](reproduction.md), which is diagnostic evidence only.
- Scoped backend/agentctl guidance and nearby existing tests. For browser work, read web guidance and the e2e and mobile-parity skills.

## Results

Implemented on 2026-10-18.

- RED: prefix validation rejected none of the new invalid-reference cases. The new preparation tests failed on missing wrapper execution and runtime-path expansion.
- GREEN: launchwrapper, managedruntime, and cliflags package tests passed with `go test -trimpath`.
- The required controller suite failed in the two managed OpenCode install cases. An overlay restoring both changed existing source files from HEAD reproduced those failures.
- `go test -trimpath ./internal/agent/settings/controller -skip TestOpenCodeInstallPreparesSelectedManagedRuntime -count=1` passed.
- Changed-file lint passed with the repository-pinned golangci-lint v2.9.0 installed under `/tmp/kandev-wrapper-tools`.

The unrelated controller baseline failure remains a validation limitation. No production caller uses wrapper preparation yet.

### PR review remediation

Keep shared preparation under `internal/common`. Verify extensionless npm commands select their Windows executable shim, including in native Windows CI.
