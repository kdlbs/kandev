---
created: 2026-08-30
status: in_progress
requirements:
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-001
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-002
system_design:
  - ../../specs/agents/system-design/guarded-tty-execution.md
legacy_specs: []
---

# Implementation Plan: Guarded TTY Execution

## Overview

Implement the bridge contract first, then add Kandev's capability-gated tool,
execution binding, audit, and nested dispatch. Pin only a reviewed bridge
release, and finish with a real guarded-agent receipt and unchanged non-TTY
delivery evidence.

## Scope

### In scope

- One bounded Codex ACP TTY tool for ordinary task agents.
- Exact bridge capability negotiation and App Server `command/exec` dispatch.
- Trusted task/execution binding, durable audit, denials, and bounded output.
- Preservation of deployment repair `6fcc88f` and ordinary non-TTY delivery.

### Out of scope

- Interactive or long-lived terminal sessions.
- Host-side execution or other ACP providers.
- Moving deployment-only bubblewrap policy into public Kandev source.

## Technical approach

### Upstream bridge

Add a versioned underscore extension to `agentclientprotocol/codex-acp`. It
derives cwd and sandbox state from the active session, generates process IDs,
forces `tty: true`, correlates App Server deltas, applies fixed bounds, and
returns one structured receipt.

### Kandev tool and routing

Add a backend-owned allow capability plus a live adapter support gate. Extend
the execution-bound agentctl stream with one guarded-TTY action. The backend
uses trusted MCP identity and current lifecycle state; agentctl invokes only an
adapter with an exact negotiated extension.

### Audit

Persist a typed task-message audit claim before dispatch and finalize it by
attestation ID. Return the same ID in the MCP result. Retain only bounded output
metadata/digest in the audit.

## Tests

- `AC-AGENTS-GUARDED-TTY-EXECUTION-001.1` through `.3`: MCP profile, catalog,
  capability, and schema unit tests.
- `AC-AGENTS-GUARDED-TTY-EXECUTION-001.4` through `.6`: bridge unit/integration
  tests and Kandev nested-stream tests.
- `AC-AGENTS-GUARDED-TTY-EXECUTION-002.1` through `.5`: scope, lifecycle,
  denial, audit repository, redaction, and receipt-validation tests.
- `AC-AGENTS-GUARDED-TTY-EXECUTION-002.6` and `.7`: concurrent non-TTY
  regression and deployment-contract coexistence evidence.

## E2E tests

A credentialed disposable task runs an ordinary guarded Codex ACP agent. The
agent discovers and selects the tool, runs both descriptor tests plus `stty`,
`pwd`, and `git status`, and yields exit zero with linked model/audit/dispatch
IDs. A denial run proves zero App Server dispatch, and a simultaneous ordinary
command proves prompt non-TTY completion.

## Work orders

- [ ] [Task 01: Add the codex-acp guarded TTY bridge](task-01-codex-acp-bridge.md)
- [x] [Task 02: Add Kandev guarded TTY routing and audit](task-02-kandev-routing-audit.md)
- [ ] [Task 03: Pin, verify, and document guarded TTY execution](task-03-integration-evidence.md)

## Verification results

- Exact Kandev MCP, adapter, lifecycle, audit, and denial focused suites pass.
- The affected guarded-TTY race suite passes, including simultaneous ordinary
  ACP prompt traffic and execution reset/restart/rebind serialization.
- `golangci-lint run ./... --new-from-rev=4d8763e4d --timeout=5m` reports zero
  issues.
- The upstream bridge implementation and real App Server PTY probe are in
  `agentclientprotocol/codex-acp#451`; upstream workflow approval, merge, and a
  released package remain pending.
- The Kandev managed runtime remains on codex-acp `1.6.0`. No unreleased branch
  or commit is pinned, so the credentialed ordinary-agent acceptance run is not
  yet claimable.

## Risks

- The bridge release and PR are an external dependency.
- Nested agentctl requests can deadlock without full-duplex ownership tests.
- Raw output can contain secrets and must not be duplicated into audit logs.
- The real-agent E2E requires credentials and a task-owned guarded runtime.
