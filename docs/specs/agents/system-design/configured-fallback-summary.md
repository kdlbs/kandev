---
status: current
system: agents
requirements:
  - REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001
created: 2026-09-08
owners:
  - kandev
---

# Configured fallback summary system design

## Context and boundaries

The agent system owns the saved `AgentProfile` model policy. The Agents settings
page presents that policy as a read-only summary; it does not consult executor
catalogs or runtime session state. Runtime precedence remains defined by the
[no silent model fallback design](no-silent-model-fallback-01.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001` | Profile-row presentation and responsive behavior |

## Profile-row presentation

`apps/web/components/settings/agents/agent-profiles-section.tsx` remains the
shared renderer for profile rows on the Agents index and grouped agent cards.
The canonical `AgentProfile` fields `requireExactModel`/`require_exact_model`,
`autoFallback`/`auto_fallback`, and `fallbackModel`/`fallback_model` are already
normalized at the API boundary.

A small pure presentation helper shall classify the profile into `exact`,
`none`, `next`, or an explicit model identifier using this precedence:

1. `require_exact_model` true -> `exact`.
2. Otherwise, `auto_fallback` true -> `next`.
3. Otherwise, a non-empty `fallback_model` -> that model identifier.
4. Otherwise -> `none` (the executor-default state).

The row shall render the existing model `Badge` first and the new fallback
`Badge` immediately after it. The fallback badge may appear alongside the
existing mode badge. The model identifier is interpolated unchanged; the
helper must not trim, validate, or compare it against advertised models.

The translation namespace `agents` owns separate localized labels for the exact,
executor-default, and automatic states plus an interpolated explicit-model
label. No module-scope translation call is permitted.

## Responsive and accessibility behavior

The row keeps its existing flex-wrap metadata layout and link/action hit areas.
The additional badge is content-only: desktop retains the current row anatomy,
and phone-sized layouts wrap the badges instead of adding a second scroll owner
or changing navigation. Existing profile-row links remain the accessible
navigation target; the fallback badge is non-interactive and does not obscure
that target.

The nearest shipped mobile exemplar is the existing profile-row layout covered
by `mobile-agent-profile-layout.spec.ts`; its flex wrapping and horizontal
overflow assertion remain the geometry baseline. A focused mobile E2E assertion
shall verify the fallback text in the same row and preserve the no-overflow
contract.

## Verification boundaries

Unit coverage tests the pure classification helper for exact, executor-default,
automatic, and explicit profiles, including exact precedence when dormant
fallback values are saved and automatic precedence when exact mode is off.
Component coverage verifies ordering after the model badge and opaque model
rendering. Desktop and mobile Playwright coverage verifies the rendered row from
real saved profile data and confirms the phone layout remains usable.

## Related design

- [No Silent Model Fallback Part 1](no-silent-model-fallback-01.md)
- [No Silent Model Fallback Part 2](no-silent-model-fallback-02.md)
