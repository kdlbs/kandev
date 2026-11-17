---
id: "04-update-codex-acp-default"
title: "Update the Codex ACP default"
status: done
wave: 4
depends_on:
  - "03-document-and-prove-upgrade-behavior"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-RUNTIME-UPDATES-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-002.1
  - AC-AGENTS-RUNTIME-UPDATES-002.5
system_design:
  - ../../specs/agents/system-design/runtime-default-activation.md
  - ../../specs/agents/system-design/runtime-updates-01.md
---

# Task 04: Update the Codex ACP Default

## Scope

Change the embedded `@agentclientprotocol/codex-acp` default from `1.13.1`
to `2.2.2`. The old adapter's model list omits `gpt-6.1-sol`, even when the
globally installed Codex CLI advertises it. The new adapter advertises the
model and successfully executes a prompt with matching runtime usage metadata.

Owned files are the managed-runtime version catalogue, this work order, and
the parent plan. Native Codex, other runtimes, and selection policy are outside scope.

## Acceptance

- Default Codex ACP commands resolve exactly `2.2.2` through the existing catalogue.
- Managed-runtime command and default-generation tests pass.
- A live probe verifies model discovery, tool execution, and session resume.
- Existing active processes retain their runtime under the established upgrade contract.

## Verification

From `apps/backend`:

```bash
go test -trimpath ./internal/agent/agents -run 'Test(Codex|ManagedNPM)' -count=1
go test -trimpath ./internal/agent/managedruntime -count=1
```

Use `acpdbg` with the exact `2.2.2` package in a temporary workspace. Verify
`gpt-6.1-sol[high]`, a harmless shell command, and `session/load` of the probe's
conversation. Inspect protocol results and runtime model usage, not model self-identification.

From the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks and documentation

This is an upstream major-version upgrade. Live tool and resume checks complement
the existing command tests. The default-generation contract applies the new default
on startup and permits a later operator rollback. Running processes remain unchanged.

No public docs change is needed. The existing agent how-to guide describes managed
versions and rollback without duplicating the catalogue's exact version values.

## Results

- Focused Codex and managed-runtime tests passed. Documentation catalogue and
  specification checks passed, as did `git diff --check`.
- The managed `2.2.2` adapter advertised `gpt-6.1-sol[high]` and completed a
  prompt. Runtime usage metadata confirmed `gpt-6.1-sol`.
- A conversation created with `1.13.1` resumed through the managed `2.2.2`
  adapter. It recalled the earlier marker and completed `pwd` in the same
  temporary workspace. The resumed turn's usage metadata reported `gpt-6.1-sol`.
- The broader two-package test command passed `managedruntime`, but four
  unrelated agent tests failed on this NixOS host. Claude's credential test
  omits PATH. Devin, Goose, and Muse installer tests use `/usr/bin:/bin`.
  Those child environments cannot find `mkdir` or `mktemp`. The focused
  Codex and managed-runtime commands above avoid those unrelated tests.
