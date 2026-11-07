---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
legacy_specs: []
---

# Implementation Plan: Start model transport failure

## Overview

A startup `SetModel` can run after the agent stream is already gone, for
example when the agent is torn down between `session/new` and the model step.
The strict model branches wrapped that failure as
`model_selection_failed`/`application_failed`, so the session error card named
the requested model as the cause of a transport or teardown race.

## Scope

In scope: `apps/backend/internal/agent/runtime/lifecycle/start_model.go` and its
permanent tests.

Excluded: auto-fallback and provider-default continuations, method-not-found
handling, permission-mode application, and any change to how the generic
bootstrap cause is presented.

## Technical approach

Use `errors.Is` with `agentctl.ErrAgentStreamNotConnected`, cancellation, and
deadline identities. The three fail-closed SetModel branches (requested model,
unique variation, advertised fallback) return proven host-stream loss unchanged.
Opaque agentctl/ACP replies remain application failures even when their text
mentions transport loss. The SDK does not preserve origin for every local write
failure, so a `RequestError` alone cannot attest a provider reply.

## Work orders

- [x] [Task 01: Pass startup model transport failures through](task-01-start-model-transport-failure.md)

## Changed-path coverage

The work order owns `start_model.go`, its permanent tests, the startup transport
amendment in the session recovery failures requirement, and the matching note in
the startup failure explanations design.

## Review remediation scope and limits

The transport classifier requires typed host evidence. This avoids turning a
provider rejection into a generic failure based on its text. It intentionally
leaves opaque remote ACP failures on the existing application-failure path;
fully distinguishing those requires origin evidence from the SDK boundary.
No agentctl wire change, SDK dependency change, or recovery-policy change is
included. The contributor commit and original generic bootstrap presentation
remain intact.

Coverage includes all fail-closed policy branches, real backend WebSocket
startup/native resume, context reset, and Kubernetes model reapplication.
Transport loss closes the actual client while SetModel is pending; a successful
startup control proves that prompt dispatch is reachable in the same fixture.
