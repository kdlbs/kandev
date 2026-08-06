---
id: "04-attachment-hub"
title: "Task-host attachment hub"
status: done
wave: 2
depends_on: ["03-task-host-supervisor"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 04: Task-Host Attachment Hub

## Acceptance

- Multiple downstream attachments share one initialized task-host peer; request IDs, cancellations,
  responses, and stale generations cannot cross attachments.
- One canonical URI is open upstream. Duplicate opens do not rewind text, changes are versioned in
  arrival order, the final detach/close releases documents, and no attachment transition stops the
  language server.
- New/reconnecting attachments receive generation, workspace metadata, capabilities, and cached
  diagnostics without sending initialize/shutdown/exit.

## TDD sequence

1. Add failing hub tests with two in-memory attachments for ID collision/remapping, cancellation,
   stale response rejection, notification fanout, diagnostic replay, and detach cleanup.
2. Add failing document-broker tests for duplicate open, stale second open, interleaved changes,
   monotonically increasing versions, save ordering, final close, and disconnect release.
3. Implement the attachment handshake and allowed feature-routing boundary. Reject lifecycle and
   unsupported client messages without terminating the task-host generation.
4. Join every attachment reader/writer on hub/manager close and run race/leak repetitions.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api -run 'Test(Attachment|Hub|Document|LSPAttach)'
cd apps/backend && go test -race ./internal/agentctl/server/lsp ./internal/agentctl/server/api
cd apps/backend && go test ./internal/agentctl/server/lsp -run 'Test(Attachment|Hub|Document)' -count=20
```

## Files likely touched

- `apps/backend/internal/agentctl/server/lsp/attachment.go`
- `apps/backend/internal/agentctl/server/lsp/hub.go`
- `apps/backend/internal/agentctl/server/lsp/documents.go`
- `apps/backend/internal/agentctl/server/lsp/attachment_test.go`
- `apps/backend/internal/agentctl/server/lsp/hub_test.go`
- `apps/backend/internal/agentctl/server/lsp/documents_test.go`
- `apps/backend/internal/agentctl/server/api/lsp.go`
- `apps/backend/internal/agentctl/server/api/lsp_test.go`
- `apps/backend/internal/lsp/protocol/message.go`
- `apps/backend/internal/lsp/protocol/message_test.go`

## Dependencies

Task 03 owns the process, upstream peer, generation, workspace metadata, and snapshots.

## Parallelism

Sequential because it mutates the task-host manager and protocol package from Task 03.

## Inputs

- Spec: task-host multiplexer and shared document synchronization.
- Current frontend `JsonRpcConnection`, `lsp-document-sync`, file-URI, and provider request shapes.
- Existing protocol 16 MiB frame limit and process-manager teardown rules.

## Output contract

Report routing/document invariants, RED/GREEN results, race/leak evidence, and any deliberately
unsupported server/client method. Update task/plan status and actual files.

## Results

Implemented a generation-bound task-host attachment hub. Each downstream request retains its local
JSON-RPC ID while the single upstream peer allocates a distinct task-host ID; per-attachment
pending maps route responses and cancellation only to the source attachment. Detach cancels and
joins pending work, drops late responses, and releases document references without changing
runtime policy or process state. Restart closes the old generation hub before replacement.

The first frame now carries `attached`, language, generation, workspace URI/folders, and initialized
capabilities. Cached diagnostics replay after that frame; safe server notifications fan out to all
current attachments. Lifecycle/configuration messages (`initialize`, `initialized`, `shutdown`,
`exit`, and `workspace/didChangeConfiguration`) are task-host-owned and rejected without closing a
healthy attachment. Server requests remain centrally handled by the Task 03 peer.

Added a synchronized canonical document broker. First open sends upstream version 1; duplicate
opens only add attachment references and cannot rewind text. Accepted full/incremental changes are
applied to the canonical overlay and receive monotonically increasing upstream versions in arrival
order. Save respects server capability and omits stale optional text. Final close/detach sends one
`didClose`; closing the last attachment leaves the hub and language server alive.

TDD and verification evidence:

- RED: hub/document types were undefined and `/attach` returned 404.
- GREEN: colliding IDs, cancellation, detach/late-response discard, stale generation rejection,
  restart closure, diagnostic replay, notification fanout, lifecycle rejection, duplicate/stale
  open, interleaved versioning, save filtering, final close, and non-owning disconnect pass.
- `go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api -run
  'Test(Attachment|Hub|Document|LSPAttach)'` — pass.
- `go test -race ./internal/agentctl/server/lsp ./internal/agentctl/server/api` — pass.
- `go test ./internal/agentctl/server/lsp -run 'Test(Attachment|Hub|Document)' -count=20` — pass
  in 0.392s with package `goleak` verification.
- Full `go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api` — pass.

Actual files added: `server/lsp/{attachment,hub,documents}.go` and hub/document tests. Actual files
updated: peer raw-call/cancellation support, runtime fanout/hub ownership, manager generation-bound
Attach, task-host WebSocket attach route/tests, this task, and parent plan.

PR remediation on 2026-08-06 added an atomic attachment publication barrier: handshake and cached
diagnostics are queued before an attachment becomes visible to live fanout. The document broker now
tracks each attachment's own text baseline, applies incremental ranges against that baseline, and
sends a full canonical replacement upstream, so a divergent duplicate open cannot corrupt ranges.
Focused hub/document tests and the task-host race suite pass.
