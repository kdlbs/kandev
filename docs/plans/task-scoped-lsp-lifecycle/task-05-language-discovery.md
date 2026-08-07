---
id: "05-language-discovery"
title: "Bounded language discovery"
status: done
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

Added registry-owned, read-only discovery signals for TypeScript/JavaScript, Python, Go, Rust, and
Kotlin. Agentctl scans file names/extensions only; it never reads manifest contents, resolves a
project binary, invokes an installer, starts a process, or initializes a language server.

The scanner starts from the existing task-host root plus validated repository subpaths, rejects
absolute/traversing/symlinked roots, deduplicates directories, sorts directory entries and final
languages, and skips dependency/VCS/build caches. Directory handles are checked against the
original `Lstat` identity before reading. Symlink directory entries are never traversed.

Every scan enforces the two-second, 10,000-entry, depth-six defaults. Entry/depth exhaustion,
deadline, cancellation, invalid roots, and unreadable roots return deterministic
complete/partial/unavailable state, truncation, reasons, entry count, and UTC scan time. Added an
authenticated read-only agentctl route and runtime-tier client method; the API test verifies its
process list remains empty.

TDD and verification evidence:

- RED: scanner/result/client methods were undefined and `/api/v1/lsp/discovery` returned 404.
- GREEN: all manifest/extension signals, duplicate and reordered roots, empty trees, ignored dirs,
  symlink loop/escape, entry/depth bounds, canceled and expired contexts, missing roots, route
  process safety, and authenticated client decoding pass.
- `go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api
  ./internal/agent/runtime/agentctl -run 'Test(Discover|Discovery|LSPDiscovery)'` — pass.
- `go test -race ./internal/agentctl/server/lsp -run 'Test(Discover|Discovery)'` — pass.
- Full `go test ./internal/lsp/... ./internal/agentctl/server/lsp
  ./internal/agentctl/server/api ./internal/agent/runtime/agentctl` — pass.

Actual files added: shared discovery result and task-host scanner/tests. Actual files updated:
installer registry signal metadata, task-host discovery route/tests, agentctl client/tests, this
task, and parent plan. Environment selection/persistence remains scoped to Tasks 06–07.
