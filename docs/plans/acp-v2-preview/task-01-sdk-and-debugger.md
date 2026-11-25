---
id: 01-sdk-and-debugger
title: Add ACP v2 support to acpdbg
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-ACP-V2-004
acceptance_criteria:
  - AC-AGENTS-ACP-V2-004.1
  - AC-AGENTS-ACP-V2-004.2
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
---

# Add ACP v2 support to acpdbg

Make the wire version selectable in the raw Kandev debugger.

## Scope

- Capture the requested and selected versions. Read the v2 initialize response
  shape before session traffic and report failures in the capture.
- Add `--protocol-version 1|2` to `acpdbg`. With v2, record selection and
  distinguish an acknowledgement from terminal state. Support the v2 replay
  path and the HTTP MCP sentinel.

The SDK worker has a separate package in `acp-go-sdk`. The raw debugger
can run without that change.

## Acceptance

1. The debugger defaults to v1 and produces the requested v2 wire frames.
   It identifies a legacy payload that claims v2.
2. The debugger verifies v2 acknowledgement, completion, replay, and MCP
   attachment against the relevant fixture.

## Verification

Run the debugger commands from `kdlbs-kandev`:

```bash
GOWORK=off make -C apps/backend build-acpdbg
(cd apps/backend && GOWORK=off go test -trimpath ./cmd/acpdbg ./internal/agent/acpdbg -count=1)
apps/backend/bin/acpdbg probe --protocol-version 2 --timeout 45s claude-acp
```

Run the documentation and diff checks from `kdlbs-kandev`.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check origin/main
```

## Files and dependencies

Likely files are `apps/backend/cmd/acpdbg/` and
`apps/backend/internal/agent/acpdbg/`. Update
`apps/backend/cmd/acpdbg/README.md` and the `acp-debug` skill.
The task workspace captures provide versioned examples for the fixtures.

## Results

- `make -C apps/backend build-acpdbg` passed.
- `go test -trimpath ./cmd/acpdbg ./internal/agent/acpdbg -count=1` passed.
  The command fixture covers the v1 default, both incompatible initialization
  responses, acknowledgement order, typed configuration, replay, and HTTP MCP.
- `acpdbg probe --protocol-version 2 --timeout 45s claude-acp` passed with
  Claude ACP 0.109.0 in an isolated home with an npm project directory.
- `python3 scripts/list-docs.py validate` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check origin/main` passed.
