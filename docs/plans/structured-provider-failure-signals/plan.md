---
created: 2026-10-20
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-03.md
legacy_specs: []
---

# Implementation Plan: Classify structured provider failure signals

## Overview

A terminal ACP prompt error already carries an allowlisted JSON-RPC code and an
adapter-declared error kind, but every consumer classified it from the message
text alone. When an adapter reports a rate limit or overload only through those
structured fields and the SDK flattens the message to `Internal error`, the
failure lost its meaning: the ACP adapter did not retain the turn as provider
capacity and the Kanban, Office, and lifecycle consumers saw an unknown error.
One sequential work order carries the explicit structured fields through the
projection and feeds them to the shared classifier.

## Scope

In scope: the `routingerr` classifier input and structured mapping, the
`streams.ProviderError` HTTP status and opaque provider code fields with a
`RoutingInput` helper, the ACP scalar metadata extractor, and the existing
consumers that classify a provider diagnostic (ACP retained-turn and capacity
continuation, Kanban, Office scheduler and failure handling, retained turn
failure messages, provider quota metadata, and lifecycle diagnostic
correlation).

Excluded: new provider text rules, retry policy values, candidate ordering,
parsing of response bodies or headers, and any change to the identity,
diagnostic-correlation, output, or effect gates.

## Technical approach

`routingerr.Input` gains the ACP code, error kind, and opaque provider code.
`classifyStructured` maps only signals with known semantics: an HTTP error
status, the ACP authentication-required code, and adapter-namespaced kinds.
`ProviderError.RoutingInput` builds the input from a valid projection so every
consumer passes the same fields, and diagnostic correlation lends metadata only
to the diagnostic whose normalized text matches.

## Work orders

- [x] [Task 01: Classify structured provider failure signals](task-01-structured-provider-failure-signals.md)

## Changed-path coverage

The work order owns the backend files above, the permanent tests beside them,
the new acceptance criterion `.32` and the updated `.22` in
`docs/specs/platform/requirements/provider-error-recovery.md`, and the updated
allowlisted-metadata section of
`docs/specs/platform/system-design/provider-error-recovery-03.md`.

## Verification

- `adapter_prompt_failure_structured_test.go` exercises the ACP retained-turn
  decision with structured-only failures and negative namespace, wrapper,
  fractional-status, and opaque-code cases.
- `routingerr/structured_test.go` covers the classifier mapping and the
  normalized-error passthrough.
- Existing `routingerr`, `agentctl`, Office, and orchestrator package tests.

## Risks

- A provider kind read outside its adapter namespace could relabel an
  unrelated failure; kinds are matched only for the declaring adapter.
- An unmapped HTTP status relabelled by prose could report authentication for
  a gateway error; a valid status without a mapping stays unknown.
