---
created: 2026-10-27
status: implemented
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-001
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
legacy_specs: []
---

# Implementation Plan: Disabled Custom Utility Admission

## Overview

Reject new execution of a disabled custom utility at the shared preparation boundary, before any template or profile resolution or call creation. One sequential work order covers the service, HTTP contract, and independently authored regression evidence.

## Scope

In scope: the custom utility's saved enabled state, typed rejection, HTTP response, session-bound and sessionless execution, compatibility controls, and public documentation of the resulting behavior.

Out of scope: built-in policy, settings save/refresh, UI toggles, schemas or migrations, cancellation, other validation or profile error status changes, and plugin host profile selection. No rendered UI changes; desktop and mobile callers share the same backend outcome.

## Technical approach

`internal/utility/service/service.go` checks `!agent.Builtin && !agent.Enabled` immediately after `GetAgentByID`, returning `ErrAgentDisabled`. `internal/utility/handlers/handlers.go` maps it to HTTP 400 with `utility agent is disabled` and no `call_id`. This retains the existing 404 for missing utilities and existing profile eligibility and failure behavior. The lookup is the admission snapshot: no update serialization or recheck of already admitted calls. Built-in rows pass through unchanged, including false-enabled inherited defaults.

Public docs assessment: `docs/public/developer-tools.md` is a user-facing how-to for custom utilities and currently describes disabled profiles, not disabled utilities. During implementation add a concise sentence explaining that disabling a custom utility blocks new calls until re-enabled and does not cancel admitted calls. No public docs edit is made in the design-only phase.

## Tests

For AC-AGENTS-UTILITY-AGENT-PROFILES-001.9, independently author `TestPreparePromptRequestDisabledCustomAdmission` in `apps/backend/internal/utility/service/admission_test.go`: verify typed rejection precedes profile and dynamic route resolution, enabled custom succeeds, disabled built-in retains inherited behavior, and a previously prepared request is not invalidated. Inspect the placement before template resolution; the concrete template engine has no injectable observer. Update existing test fixtures that intend executable custom rows to set `Enabled: true`.

Independently author `TestExecuteDisabledCustomAdmission` in `apps/backend/internal/utility/handlers/admission_test.go`, using registered HTTP routes, real SQLite store, real service/controller and eligible explicit profile resolution with counted profile-aware executor stubs. In both session modes create, disable, reread, and execute a custom utility; assert HTTP 400, error text, absence of `call_id`, no new call row and no dispatch. Controls cover enabled, disabled then re-enabled, nonexistent utility, and disabled built-in inheritance, preserving response and call behavior. Include ineligible profile control and a synchronized admitted-call test proving later disable does not cancel dispatch. Tests must not use ROOT's private qualifier or historical fixture source.

## E2E tests

The externally observable API is exercised through registered HTTP and SQLite in both transport modes. No rendered UI or mobile interaction changes; browser E2E would add no distinct coverage for this backend admission boundary.

## Work orders

- [x] [Task 01: Reject disabled custom utility execution](task-01-admission.md)

## Verification results

Focused RED reproduced disabled admission in both modes and unexpected profile/route resolution. Focused GREEN and both owning Go packages passed with race detection. Changed-Go lint passed with zero issues. Catalog validation, specification lint, public-doc tests (63 passed), and public-doc validation (54 pages) passed. PR documentation coverage preflight and final diff check are recorded in the work order.

## Risks

Existing tests may seed executable custom rows with the zero-value enabled field and need explicit intent. A concurrent disable after admission does not revoke the already admitted call; the design deliberately does not claim atomic serialization. The historical profile-error status wording in the prior plan is not a mandate to broaden this repair.
