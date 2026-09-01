---
id: "05-review-remediation"
title: "Close scoped CI review findings"
status: done
wave: 4
depends_on: ["04-verification-and-docs"]
plan: "plan.md"
spec: "../../specs/integrations/scoped-coordinator-ci-runs.md"
---

# Task 05: Close scoped CI review findings

Atomically replace generation-tracked grants and persist terminal request/audit
outcomes. Classify provider reads before policy handling, preserve retry and
request metadata, and return the complete non-secret repository, event,
principal, provider-request, and timing receipt through MCP success and typed
failure responses.

Acceptance conditions:

- Concurrent grant replacements retain one active grant with monotonically
  increasing generations, and a failed replacement rolls back revocation.
- Terminal request state and its terminal audit row commit or roll back
  together.
- Provider rate limits remain retryable with reset metadata, and receipts and
  audit rows contain the approved non-secret provider identity without secrets.

Verification:

- `go test -race ./internal/github ./internal/mcp/handlers ./internal/mcp/server`
- `go test ./internal/backendapp ./pkg/websocket`
- `golangci-lint run ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp --timeout=5m`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

Results:

- Atomic replacement, terminal rollback, provider-rate reconciliation, and
  complete identity tests pass under normal and race execution.
- The full backend linter reports zero issues. Specification lint, 61 public
  documentation tests, and validation of 41 published pages pass.
- `TEST_RUNTIME=TRANSIENT`: isolated SQLite stores and fake GitHub transports
  exercised the behavior; no reusable service, shared data, or live consumer
  run was required or mutated.
