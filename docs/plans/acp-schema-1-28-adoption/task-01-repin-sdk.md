---
id: "01-repin-sdk"
title: "Repin the ACP SDK to fork main"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-001
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-001.1
  - AC-AGENTS-ACP-SESSION-EVENTS-001.2
  - AC-AGENTS-ACP-SESSION-EVENTS-001.3
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 01: Repin the ACP SDK to fork main

## Summary

Point the backend at the squash-merged fork `main` commit `70cc752b95c6`, replacing the PR-head pin. Confirm that the prompt-error origin code survived the squash. Existing PR #4983 compatibility fixes stay in place.

## In scope

- `go.mod` and `go.sum` repin to `v0.13.6-0.20261101161427-70cc752b95c6`.
- The immutable pin recorded in the existing error-origin work order.
- A missing-name config option regression if the existing tests do not already prove AC 001.3.

## Out of scope

- Capability advertisement or any new event handling.

## Acceptance

- `apps/backend/go.mod` names commit `70cc752b95c6`, and `RequestError.IsRemoteResponse` still compiles against it.
- The full backend suite passes with the new pin.

## Verification

```bash
(cd apps/backend && go mod edit -replace github.com/coder/acp-go-sdk=github.com/kdlbs/acp-go-sdk@v0.13.6-0.20261101161427-70cc752b95c6 && go mod tidy)
grep -q 70cc752b95c6 apps/backend/go.mod
(cd apps/backend && go build -trimpath -tags fts5 ./...)
(cd apps/backend && go test -trimpath -tags fts5 -p=2 ./...)
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `docs/plans/*/task-01-sdk-error-origin.md`

## Dependencies

None

## Risks

- The first concurrent full-suite run previously flaked in `common/subproc` under load; `-p=2` avoids that.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/acp-session-events.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Passed:

- `go build -trimpath -tags fts5 ./...`
- `go test -trimpath -tags fts5 -p=2 ./...`
- `go test -trimpath -count=1 github.com/coder/acp-go-sdk -run TestNewSessionResponse_DropsMalformedConfigOptionNames`
- `go test -trimpath -count=1 ./internal/agentctl/server/utility -run TestProbeACPSessionWithContextRejectsProviderWithoutModelSelection`
- `go test -trimpath -count=1 ./internal/agentctl/server/adapter/transport/acp -run TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity`
