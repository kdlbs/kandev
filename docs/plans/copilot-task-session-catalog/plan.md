---
status: in_progress
system: platform
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
system_design:
  - "../../specs/platform/system-design/mcp-session-observability.md"
created: 2026-09-21
updated: 2026-09-21
owners:
  - Kandev
---

# Implementation Plan: Copilot Task Session Catalog and Launch Receipts

## Purpose

Make session-scoped MCP catalog observations and launch receipts attributable to
the immutable startup attempt that produced them. This plan covers only the
internal evidence path: agentctl stream payloads, lifecycle ownership stamping,
and orchestrator persistence. It does not change provider authentication,
profile-generation guards, executor admission, or queued-message authorization.

## Delivery scope

- Carry the startup generation on every MCP attachment attempt and evidence
  event after lifecycle accepts the event under its startup-attempt lease.
- Bind a launch receipt to an MCP attachment only when session, execution, and
  startup generation all match the current bounded receipt.
- Preserve current and two historical diagnostic attempts while preventing a
  delayed old attachment from rewriting the replacement receipt.
- Prove the behavior through the production lifecycle and orchestrator event
  handlers with deterministic fresh, resume, and delayed-event fixtures.

## Work orders

- [x] [Task 01: Fence launch receipt MCP attachment binding](task-01-fence-launch-receipt-mcp-attachment.md)

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(HandleAgentEventStampsMCPAttachmentIdentity|HandleSessionMCPAttachmentEventRejectsDelayedSameExecutionAttachment)' -count=1`
- `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(HandleAgentEventStampsMCPAttachmentIdentity|HandleSessionMCPAttachmentEventRejectsDelayedSameExecutionAttachment)' -count=1`
- Run the repository `pr-docs.cjs` coverage evaluator against the Git-backed
  changed-file snapshot before publication.
