---
created: 2026-08-30
status: done
requirements:
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-001
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-002
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-003
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-004
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-005
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-006
system_design:
  - ../../specs/integrations/system-design/scoped-coordinator-ci-runs.md
legacy_specs: []
---

# Scoped coordinator CI runs implementation plan

## Goal

Implement the server-owned trust boundary in independently verifiable layers:
durable authority/idempotency, GitHub App Actions transport, policy service,
and MCP composition/documentation. The delivery package implements the
[scoped coordinator CI run requirements](../../specs/integrations/requirements/scoped-coordinator-ci-runs.md)
per the [system design](../../specs/integrations/system-design/scoped-coordinator-ci-runs.md).

## Work orders

1. [Task 01: Persist scoped grants and idempotent CI requests](task-01-persistence.md)
2. [Task 02: Add server-owned GitHub Actions operations](task-02-provider.md)
3. [Task 03: Enforce policy and expose the closed MCP request](task-03-policy-and-mcp.md)
4. [Task 04: Verify fixture policy and document rollout](task-04-verification-and-docs.md)
5. [Task 05: Close scoped CI review findings](task-05-review-remediation.md)

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
6. Remediate review findings for atomic grant replacement, atomic terminal
   audit persistence, provider-read classification, and complete non-secret
   receipt/audit identity.

## Verification

- `go test -race ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp`
- `golangci-lint run ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp --timeout=5m`
- Public docs coverage and link checks described by `docs/public/README.md`.

No submodule changes or long-running runtime are required. Live consumer PRs
are deliberately excluded from Work verification and remain queued for a
reviewed rollout.

## Results

- Exact-scope grant replacement now revokes and increments its generation in
  one transaction, including concurrent replacement and rollback coverage.
- Terminal request/audit persistence is atomic. Provider read and mutation
  errors preserve rate-reset, request, URL, and non-secret App principal
  identities without reopening a possibly sent mutation.
- MCP success and typed failure responses return the durable repository, PR,
  head, source/result run, workflow, event, evidence, idempotency, provider,
  and timing receipt.
- Normal and race-focused backend suites, full backend lint, specification
  lint, and all public documentation validators pass.
