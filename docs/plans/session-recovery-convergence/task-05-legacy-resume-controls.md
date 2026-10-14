---
id: "05-legacy-resume-controls"
title: "Expose guarded Resume for legacy delivery fences"
status: completed
wave: 5
depends_on:
  - 04-prepared-prompt-admission
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 05: Legacy delivery Resume controls

## Evidence and scope

A native-loaded, idle session retained an empty generic agent-delivery block,
a SQL dispatching record and journal interrupted-unknown evidence. Its legacy
prompt binding lacked original process identity and prompt generation, so
automatic dead-owner reconciliation could not authorize retirement. Normal
explicit Resume recovered the same native conversation without replay or cursor
changes. The composer exposed Retry connection and Stop, hiding the supported
manual recovery path.

## Implementation

Offer Resume first for an empty unresolved-durable-work agent-delivery block
without active reconnecting/uncertain recovery metadata. Preserve Retry for
missing-record reconstruction and Stop. Explicit Resume continues through the
existing authenticated backend checks; no automatic fallback or weakened owner
guard is introduced. An open block outranks historical restored presentation,
and its identity invalidates pending responses for a superseded block.

Use existing localized labels and responsive recovery controls on desktop and
phone. No new action sends an old user prompt or creates a conversation.

## Validation and delivery

Focused unit regressions reproduce the missing Resume and hidden-block failures,
check action eligibility and pending-response identity, and assert a single
explicit Resume request. Desktop and phone CI scenarios seed SQL dispatching
against journal unknown, an empty block and absent metadata; recover through
the UI; verify native identity, unknown retirement and a subsequent new prompt.
Run focused frontend tests, typecheck, scoped lint and documentation validation.
No local browser/E2E tests, production edits or deployment. Commit, push, open
PR, address CI/reviews and merge the passing head in this session.

## Local results

The two behavioral regressions failed before implementation. All 44 focused
frontend tests now pass; changed-file ESLint passes with zero warnings. Catalog
and specification validation pass. Full frontend typecheck exceeded the imposed
2 GB heap cap (exit 134); CI must complete typecheck and the authored desktop/
phone E2E scenarios. No local E2E ran.

Review remediation adds a distinct localized legacy Resume notice without
claiming that an existing record is missing. It retains the no-Resume assertion
after canonical record reconstruction and checks Retry visibility alongside
Resume and Stop in the legacy browser scenario. All seven translated catalogs
and pseudo pass i18n checks. The exact PR documentation evaluator now passes
after linking requirements 006 and 008 in the parent plan and design metadata.

PR #4443 merged as ed56b752e18e with 49 passing checks, 19 skipped checks,
no failed or pending checks, and all three review threads resolved.
