---
id: chief-of-staff-01
title: Implement native orchestration persona
status: done
wave: 1
depends_on: []
plan: plan.md
spec: ../../specs/chief-of-staff/spec.md
---

Implement persona configuration, persistent conversation, bounded management
context and existing routine integration sequentially. Acceptance and risks are
in the spec and plan. Extend existing Office services rather than creating a
second scheduler. Validate backend contracts with focused go test invocations,
frontend with typecheck and targeted Vitest, and then run local E2E.

Do not mark complete until the user can converse with a persona and receive a
worker result through the complete native orchestration loop.

Completed and verified for local testing; see plan.md for final evidence and boundaries.
