---
id: "01-quiet-claude-passthrough"
title: "Make Claude passthrough quiet by default"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-PASSTHROUGH-LAUNCH-001
acceptance_criteria:
  - AC-CLI-PASSTHROUGH-LAUNCH-001.1
  - AC-CLI-PASSTHROUGH-LAUNCH-001.2
  - AC-CLI-PASSTHROUGH-LAUNCH-001.3
  - AC-CLI-PASSTHROUGH-LAUNCH-001.4
  - AC-CLI-PASSTHROUGH-LAUNCH-001.5
system_design:
  - ../../specs/cli/system-design/passthrough-launch-defaults.md
---

# Task 01: Make Claude Passthrough Quiet by Default

## Summary

Remove the forced `--verbose` argument from the built-in Claude passthrough
command. Preserve verbose output as an explicit profile CLI flag.

## In scope

- Add focused command-construction regression tests for Claude passthrough.
- Remove the hardcoded default `--verbose` argument.
- Preserve all other Claude passthrough arguments and settings.
- Document the default and the profile CLI-flag opt-in.
- Keep the settings command preview aligned with runtime CLI flags.

## Out of scope

- A dedicated verbose-output setting.
- Changes to ACP launches or other agent definitions.
- Browser tests and live Claude integration tests.

## Acceptance

- The regression test fails before the production change because the base
  command contains `--verbose`.
- The default Claude passthrough argv omits `--verbose` after the correction.
- An enabled profile CLI flag adds `--verbose` to the built command.
- The public profile reference explains the quiet default and verbose opt-in.
- The passthrough command preview includes enabled profile CLI flags.

## Verification

```bash
go test ./internal/agent/agents -run '^TestClaudeACP_PassthroughCmd_' -count=1
go test ./internal/agent/agents -count=1
go test ./internal/agent/settings/controller -run 'TestController_PreviewAgentCommand_PassthroughIncludesCLIFlags' -count=1
go test ./internal/agent/settings/controller -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run the Go commands from `apps/backend`. Run the Node commands from the
repository root.

## Files likely touched

- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/claude_acp_passthrough_test.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/controller/command_preview_test.go`
- `docs/public/agents-and-profiles.md`
- `docs/specs/INDEX.md`

## Dependencies

None.

## Risks

- The test must distinguish the built-in default from an explicit profile
  flag.
- The production edit must not change other passthrough arguments.

## Parallelism

`sequential`

## Inputs

- `docs/specs/cli/requirements/passthrough-launch-defaults.md`
- `docs/specs/cli/system-design/passthrough-launch-defaults.md`
- GitHub issue #3305 and the diagnostic reproduction from this task.

## Results

- RED: The focused regression command failed both tests before the production
  change for the expected hardcoded-flag reasons.
- GREEN: The focused regression command passed two tests after the production
  change.
- Fixup RED: The new controller preview test failed before profile CLI tokens
  were forwarded through the passthrough preview branch.
- Fixup GREEN: The preview and runtime now share the resolved CLI-token path,
  with duplicate permission tokens removed. Focused and full controller tests
  passed.
- Full agent package: `go test ./internal/agent/agents -count=1` passed 260
  tests; `go test ./internal/agent/settings/controller -count=1` passed 237
  tests.
- Specification lint and the CLI catalog checks passed after adding the
  preview requirement.
- Public documentation checks passed 61 tests and validated 41 published pages.
- `git diff --check` and `gofmt -l` passed for changed Go files.
