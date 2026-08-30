---
title: Scoped coordinator CI runs implementation plan
status: complete
spec: ../../specs/integrations/scoped-coordinator-ci-runs.md
---

# Scoped coordinator CI runs implementation plan

## Goal

Implement the server-owned trust boundary in four independently verifiable
layers: durable authority/idempotency, GitHub App Actions transport, policy
service, and MCP composition/documentation.

## Sequence

1. Add replayable SQLite tables for coordinator grants, request ledger, and
   redacted audit events. Test caller-key and semantic-key concurrency.
2. Extend App permission mapping and the bearer client with typed Actions and
   PR/workflow reads, exact provider failure classification, and no credential
   exposure.
3. Implement the policy service that resolves task/workspace/step/repository/PR,
   validates exact head/source-run identity, selects rerun-first fallback, and
   reconciles ambiguous calls.
4. Add authenticated grant management and a closed MCP tool whose server
   injects caller task/session identity. Wire it into backend composition.
5. Update public docs and coverage, run focused packages with `-race`, lint the
   touched backend packages, then commit the reviewed implementation.

## Verification

- `go test -race ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp`
- `golangci-lint run ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp --timeout=5m`
- Public docs coverage and link checks described by `docs/public/README.md`.

No submodule changes or long-running runtime are required. Live consumer PRs
are deliberately excluded from Work verification and remain queued for a
reviewed rollout.
