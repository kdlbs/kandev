---
id: "03-recovery-control"
title: "Present operator recovery on the agent detail surface"
status: pending
wave: 2
depends_on: ["02-status-mutation-client"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-RECOVERY-001
  - REQ-OFFICE-AGENT-RECOVERY-002
acceptance_criteria:
  - AC-OFFICE-AGENT-RECOVERY-001.1
  - AC-OFFICE-AGENT-RECOVERY-001.2
  - AC-OFFICE-AGENT-RECOVERY-001.4
  - AC-OFFICE-AGENT-RECOVERY-001.5
  - AC-OFFICE-AGENT-RECOVERY-001.6
  - AC-OFFICE-AGENT-RECOVERY-001.7
  - AC-OFFICE-AGENT-RECOVERY-001.9
  - AC-OFFICE-AGENT-RECOVERY-002.1
  - AC-OFFICE-AGENT-RECOVERY-002.2
system_design:
  - ../../specs/office/system-design/agent-recovery.md
---

# Task 03: Present operator recovery on the agent detail surface

## Summary

Render the recovery control and the pause-reason text on the agent detail
identity strip, so they appear on every agent sub-route. Activation calls the
status client, patches the Office agent store from the response body, and
handles the in-flight and failure paths without an optimistic write.

## In scope

- A recovery control component mounted in the agent detail layout's identity
  strip, gated on a `paused` or `stopped` status.
- Pause-reason text driven by the same store row.
- In-flight, success, and failure handling, including the store patch from the
  response body and a surfaced error on failure.
- Recovery copy in the five complete locale catalogs and the pseudo catalog.
- Component unit coverage for the status gate, the request lifecycle, and the
  accessible name.

## Out of scope

- Any backend change or new endpoint call beyond the status mutation.
- Failure-counter resets, inbox dismissal, and run re-queueing.
- Browser coverage, which Task 04 owns.
- A control for `pending_approval`, `working`, or any pause/stop direction.

## Acceptance

- The control renders for `paused` and `stopped` and renders for no other
  value, including an empty string and an unrecognized status.
- Activation issues one request while in flight, renders no status before the
  response resolves, and on success renders the server's status and clears the
  pause-reason text without a reload.
- A failed request leaves the rendered status and pause reason unchanged,
  surfaces an error, and leaves the control activatable again.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run "app/office/agents/[id]/components/agent-recovery-control.test.tsx")
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

The install line applies only in a fresh worktree where `apps/node_modules/` is
absent. Run `pnpm run i18n:zh-hant` before `i18n:check` to generate the
Traditional Chinese pair from the Simplified catalog.

## Files likely touched

- `apps/web/app/office/agents/[id]/layout.tsx`
- `apps/web/app/office/agents/[id]/components/agent-recovery-control.tsx`
- `apps/web/app/office/agents/[id]/components/agent-recovery-control.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/office.json`
- `apps/web/src/locales/pseudo/office.json`

## Dependencies

Task 02 provides the status mutation client this control calls.

## Risks

- The layout is already near the repo's component-size limits; putting the
  control inline rather than in its own component is the likely lint failure.
- The store patch needs the active workspace id, not just the agent id, because
  `updateOfficeAgentProfile` is keyed by workspace.
- The identity strip is a single non-wrapping `flex` row. Two new children need
  wrapping or they push the strip past a phone viewport.
- Five locale catalogs and the pseudo catalog gate the build; English-only copy
  fails `i18n:check`.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/agent-recovery.md`, REQ-OFFICE-AGENT-RECOVERY-001
  and REQ-OFFICE-AGENT-RECOVERY-002.
- `docs/specs/office/system-design/agent-recovery.md`, "Components and
  responsibilities" and "Control flow".
- `apps/web/app/office/agents/[id]/layout.tsx` for the identity strip.
- `apps/web/lib/state/slices/office/office-slice.ts`, `updateOfficeAgentProfile`.
- `apps/web/app/office/inbox/inbox-item-row.tsx`, `MarkFixedButton` for the
  repo's busy-button pattern.

## Results

Pending.
