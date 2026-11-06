---
id: "02-acpdbg-capabilities"
title: "Extend acpdbg capabilities and model selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-005
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-005.1
  - AC-AGENTS-ACP-SESSION-EVENTS-005.2
  - AC-AGENTS-ACP-SESSION-EVENTS-005.3
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 02: Extend acpdbg capabilities and model selection

## Summary

Let `acpdbg` advertise arbitrary client capabilities and select models on agents without `session/set_model`. Later work orders use it to recapture wire evidence.

## In scope

- `--client-capabilities` parsing, early validation, and the merge with `--air-capabilities`.
- Model selection through a `model`-category config option, with the legacy fallback.
- `apps/backend/cmd/acpdbg/README.md` and `.agents/skills/acp-debug/SKILL.md`.

## Out of scope

- Mode selection changes.
- Permission auto-approval.

## Acceptance

- An invalid JSON value fails before the child starts.
- A recorded `initialize` frame shows merged capabilities, and `_meta` from both flags survives.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./cmd/acpdbg/... ./internal/agent/acpdbg/...)
make -C apps/backend build-acpdbg
apps/backend/bin/acpdbg probe --timeout 90s --client-capabilities '{"session":{"notices":{},"compaction":{}}}' --out /tmp/acpdbg-check claude-acp
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/acpdbg/main.go`
- `apps/backend/cmd/acpdbg/main_test.go`
- `apps/backend/cmd/acpdbg/README.md`
- `apps/backend/internal/agent/acpdbg/ops.go`
- `apps/backend/internal/agent/acpdbg/runner.go`
- `.agents/skills/acp-debug/SKILL.md`

## Dependencies

None

## Risks

- The live probe needs Claude credentials; report it as skipped if they are unavailable.

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/agents/system-design/acp-session-events.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Passed:

- `go test -trimpath -count=1 ./cmd/acpdbg/... ./internal/agent/acpdbg/...`
- `make -C apps/backend build-acpdbg`
- Live Claude ACP probe with `--client-capabilities '{"session":{"notices":{},"compaction":{}}}'`; the recorded initialize frame contained both capabilities.
- `bash scripts/lint-go-changed` for changed Go files
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
