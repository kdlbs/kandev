---
id: "01-client-and-debugger"
title: "Shared native client and debugger"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-005
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-002.4
  - AC-AGENTS-CODEX-NATIVE-005.1
  - AC-AGENTS-CODEX-NATIVE-005.2
  - AC-AGENTS-CODEX-NATIVE-005.3
  - AC-AGENTS-CODEX-NATIVE-005.4
  - AC-AGENTS-CODEX-NATIVE-005.5
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 01: Shared native client and debugger

## Summary and scope

Build the shared typed client, versioned schema fixture, fake server, and standalone developer inspection workflow.
Implement the debugger commands in the agent design, including offline capture inspection and bounded post-turn observation.
Create the skill through `skill-creator` and `harness-improvement`; keep examples aligned with the actual CLI parser.

## Exclusions

No agent registration, application profile changes, or live-session attachment.

## Likely files and ownership

- New `apps/backend/pkg/codexappserver/`, including schema generation metadata and fixtures.
- New `apps/backend/cmd/codexdbg/` and `internal/agent/codexdbg/`.
- `apps/backend/Makefile`, root ignore patterns for explicit debug captures.
- New `.agents/skills/codex-app-server-debug/SKILL.md` and bounded references.
- `.agents/skills/using-agent-skills/SKILL.md`: add the precise native-debug trigger.
- Read `internal/agent/acpdbg/` for framing/capture patterns; do not change ACP behavior.

## Acceptance

1. Fake-server tests prove concurrent response correlation, server requests, string/numeric IDs, framing limits, cancellation, EOF, and process cleanup.
2. Probe performs no turn; capture is ordered, exclusive, permission-restricted, and honest about timeout/truncation. Answer files cannot implicitly approve unknown requests.
3. Offline inspection distinguishes response/turn/thread scopes, and the skill demonstrates the implemented commands without exposing raw data in routine logs.

## TDD and verification

Write the named client and debugger tests from the plan first, observe failure, then implement.
Also test every command's argument parsing and error path.
Run from the repository root:

```bash
(cd apps/backend && go test ./pkg/codexappserver/... ./internal/agent/codexdbg/... ./cmd/codexdbg/...)
make -C apps/backend build-codexdbg
apps/backend/bin/codexdbg --help
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
git diff --check
```

Schema generation must record `codex --version` and the source digest.
Do not require account credentials for automated fake-server tests.

## Risks

Internal-only events can differ by runtime build. Unknown fields must survive raw capture without becoming mandatory production fields.
The CLI owns only its subprocess tree and disposable workspace.

## Results

Implemented the shared JSON-RPC client, pinned 0.154.0 schema fixture, debugger runner, capture recorder, MCP sentinel, safe request policy, command parser, and offline scope-aware inspection. The probe path performs no thread or turn creation. The default turn path does not send an interrupt.

Validation passed:

```text
(cd apps/backend && go test ./pkg/codexappserver/... ./internal/agent/codexdbg/... ./cmd/codexdbg/...)
make -C apps/backend build-codexdbg
apps/backend/bin/codexdbg --help
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
git diff --check
```
