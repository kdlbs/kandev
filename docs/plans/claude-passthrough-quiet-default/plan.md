---
created: 2026-09-03
status: done
requirements:
  - REQ-CLI-PASSTHROUGH-LAUNCH-001
system_design:
  - ../../specs/cli/system-design/passthrough-launch-defaults.md
legacy_specs: []
---

# Implementation Plan: Claude Passthrough Quiet Default

## Overview

Remove the forced Claude verbose flag from the built-in passthrough command.
Keep verbose output available through the existing profile CLI-flags field.

The confirmed root cause is the `--verbose` token in
`NewClaudeACP().PassthroughConfig().PassthroughCmd`. The standard passthrough
builder cannot disable a base-command token, so every Claude passthrough launch
receives the flag.

## Scope

### In scope

- Make the built-in Claude passthrough command quiet by default.
- Preserve explicit `--verbose` opt-in through profile CLI flags.
- Add regression tests for both command forms.
- Document the quiet default and the verbose opt-in path.

### Out of scope

- A new profile setting or user-interface control.
- Changes to ACP launches, other agents, or Claude output rendering.
- Migration of existing agent profiles.

## Technical approach

Add a focused test file beside `claude_acp.go`. First, prove that the current
base command violates the quiet-default contract. Then remove only the
hardcoded `--verbose` token.

Use `BuildPassthroughCommand` in the opt-in test. This boundary proves that the
existing `CLIFlagTokens` path adds `--verbose` without a new setting.

Update the CLI-flags reference in `docs/public/agents-and-profiles.md`. State
that Claude passthrough uses standard output by default and that `--verbose` is
an explicit profile flag.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-CLI-PASSTHROUGH-LAUNCH-001.1` | `TestClaudeACP_PassthroughCmd_DefaultOmitsVerbose` compares the base argv with the quiet command. |
| `AC-CLI-PASSTHROUGH-LAUNCH-001.2` | `TestClaudeACP_PassthroughCmd_VerboseOptIn` builds a command with `CLIFlagTokens` and expects `--verbose`. |
| `AC-CLI-PASSTHROUGH-LAUNCH-001.3` | The focused agent package tests preserve existing command-composition coverage. |
| `AC-CLI-PASSTHROUGH-LAUNCH-001.4` | The opt-in test uses the current profile token input and requires no model change. |

## E2E tests

No browser E2E test is planned. The change has no user-interface path, and a
live Claude dependency would not give deterministic repository evidence.

The agent-package test covers the complete Kandev-owned command argv. Anthropic
owns the output that each argv mode renders.

## Work orders

- [x] [Task 01: Make Claude passthrough quiet by default](task-01-quiet-claude-passthrough.md)

## Verification results

- RED: `go test ./internal/agent/agents -run
  '^TestClaudeACP_PassthroughCmd_' -count=1` failed two tests before the
  production change. The default argv contained the forced flag, and explicit
  opt-in produced a duplicate flag.
- GREEN: The focused command passed two tests after the production change.
- Full agent package: `go test ./internal/agent/agents -count=1` passed 259
  tests.
- Public documentation: `node --test scripts/validate-public-docs.test.mjs`
  passed 61 tests, and `node scripts/validate-public-docs.mjs` validated 41
  published pages.
- `git diff --check` and `gofmt -l` passed for changed Go files.

## Risks

- A user who depended on forced verbose output must add `--verbose` to that
  profile.
- An overly broad command change can damage model, resume, permission, prompt,
  or MCP arguments. The implementation changes one base token only.
