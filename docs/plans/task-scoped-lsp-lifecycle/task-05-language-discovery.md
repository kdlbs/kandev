---
id: "05-language-discovery"
title: "Bounded language discovery"
status: pending
wave: 2
depends_on: ["03-task-host-supervisor"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 05: Bounded Language Discovery

## Acceptance

- An existing supported task host detects the registered languages across task and ordered
  repository roots without opening a file, executing/parsing a project manifest, installing a
  binary, or starting an LSP.
- Every scan enforces two seconds, 10,000 entries, depth six, ignored directories, cancellation,
  root containment, and no directory-symlink traversal; exhausted budgets return partial results.
- Results are deterministic and carry scan time/truncation evidence for backend persistence.

## TDD sequence

1. Add table-driven failing temp-tree tests for TypeScript/JavaScript, Python, Go, Rust, and Kotlin
   manifest/extension signals, multi-root ordering, duplicates, and empty trees.
2. Add adversarial tests for ignored dependency/build directories, a symlink loop/escape, max
   depth, entry budget, deadline/cancellation, unreadable paths, and partial-result truthfulness.
3. Implement the name/extension-only scanner and authenticated agentctl snapshot route.
4. Add a backend agentctl client method; prove it has no process/installer calls and does not create
   or resume a task environment.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test(Discover|Discovery|LSPDiscovery)'
cd apps/backend && go test -race ./internal/agentctl/server/lsp -run 'Test(Discover|Discovery)'
```

## Files likely touched

- `apps/backend/internal/agentctl/server/lsp/discovery.go`
- `apps/backend/internal/agentctl/server/lsp/discovery_test.go`
- `apps/backend/internal/agentctl/server/api/lsp.go`
- `apps/backend/internal/agentctl/server/api/lsp_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_lsp.go`
- `apps/backend/internal/agent/runtime/agentctl/client_lsp_test.go`
- `apps/backend/internal/lsp/installer/registry.go` only to expose existing registered language metadata

## Dependencies

Task 03 supplies the instance-owned LSP API and workspace metadata. It does not depend on the
attachment hub.

## Parallelism

Sequential in this workflow. Although scanner logic is isolated, route/client files overlap Task 03.

## Inputs

- Spec discovery bounds and executor/resource failure modes.
- `process.Manager.RepoSubpaths()` and workspace containment/security test conventions.
- Installer registry's authoritative supported language IDs; never duplicate a second language list.

## Output contract

Report detected signals, exact bound/cancellation evidence, proof of zero execution/install side
effects, and test results. Update task/plan status and actual files.

## Results

Pending.
