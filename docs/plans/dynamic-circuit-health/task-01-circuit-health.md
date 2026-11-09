---
id: "01-circuit-health"
title: "Port the coherent circuit core and readback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.1
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.2
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.3
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.4
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.5
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.6
system_design:
  - ../../specs/agents/system-design/dynamic-circuit-health.md
---

# Task 01: Port the Coherent Circuit Core and Readback

## Scope

- Existing runtime circuit admission, failure attribution, backoff/count/reset,
  registry snapshot, resolver, and orchestrator consumers.
- Existing SQLite circuit table, additive count migration, startup flag wiring,
  controller reader, HTTP handler, and backend adapter.
- Circuit-only API/validation/hook/card and the current providers dashboard;
  localized circuit copy in the existing catalogs.
- Reuse source checks only for their unchanged scope. Check the new main
  integration through permanent regression tests, current feature contracts,
  and desktop/mobile browser checks.

## Boundaries

No donor mutation, operating installation/settings change, provider call,
automatic failover, new budget policy, model-order change, new permission model,
whole-dashboard import, or new test framework. Provider-policy
limit read/display remains separate. Delivery uses a maintainer follow-up commit on the contributor branch.

## Validation and result

Fixed-profile admission and failure charging reuse `features.dynamicAgentRouting`.
No separate admission flag is added. Office defers suspended dispatches without
using retries or provider-attempt allowance. Queue waits use a targeted query,
release unknown verdicts, and clear their diagnostic under a revision guard.
Expired open circuits display as available without a past next-attempt time.

Permanent tests cover the backoff ladder, expiry and holds, durable failure
counts, captured resource identity, fixed admission, queue retention and resume,
Office deferral, read transport, validation, and hook ownership. The affected Go
suites pass with `-race`; scoped Go lint passes. Web lint, full typecheck with an
8 GiB Node heap, feature contracts, localization, and specification checks pass.
The provider-circuit Playwright case passes on `chromium` and `mobile-chrome`
against fresh backend and web builds. No PostgreSQL, real-provider, or full-suite
pass is claimed.
