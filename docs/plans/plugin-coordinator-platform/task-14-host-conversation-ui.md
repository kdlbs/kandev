---
id: "14-host-conversation-ui"
title: "Reusable workspace conversation and task-status UI"
status: complete
wave: 14
depends_on:
  [
    "02-capability-settings",
    "03-managed-lifetime",
    "04-restricted-tools",
    "05-durable-input",
    "06-workspace-observations",
    "08-execution-controls",
  ]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-009
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-009.1
  - AC-PLUGINS-COORDINATION-009.2
  - AC-PLUGINS-COORDINATION-009.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 14: Reusable workspace conversation and task-status UI

## Summary

Publish generic Host UI primitives and a managed-conversation frontend facade. Prove them with the test fixture before building a production plugin.

## Implementation checklist

- [x] Add the typed Host SDK contracts and query hooks for managed chat, task status, and usage.
- [x] Implement reusable desktop/phone chat, queue, consent, pause, and recovery surfaces.
- [x] Add fixture route coverage for task context, outcomes, instance selection, and settings.
- [x] Add the desktop and phone E2E flows and localization in all supported catalogs.
- [x] Update Host API and authoring documentation, then run required checks.

## In scope

- Add WorkspaceAgentChat and bounded canonical task-status/usage primitives to host-api, public types, SDK, and registry.
- Implement receipt-driven composer, ordered messages, reconnect, pause, exact interaction consent, and recovery. Add a host read-only retained transcript view.
- Use a fixture route for desktop split view and native phone tabs/drawers; test loading, empty, offline, revoked, unsupported, and long content.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A fixture plugin renders live managed chat and current task status using only public exports.
- Reconnect/retry does not duplicate input and human interaction/recovery controls use native consent.
- Phone and desktop retain equivalent actions with accessible focus, localization, safe areas, and no horizontal overflow.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-02: Workspace conversation

```text
Desktop: plugin navigation > Coordinator
+---------------------------------------------------------------------+
| Product workspace   [Delivery lead v] [Settings] [Pause]              |
| Outcomes: 8 completed  1 blocked  Usage: USD 2.10 measured             |
+----------------------------------+----------------------------------+
| Chat                             | Tasks  [Search] [Filters]        |
| You: follow up the review        | Blocked                          |
| Agent: proposal ready            | [Task A] Waiting for answer      |
| [Proposal: Review fix] [Approve] | Running                          |
|                                  | [Task B] Implementing            |
| Input queued: 1  [Cancel]         | Completed                        |
| [Message composer]        [Send] | [Task C] Verified                |
+----------------------------------+----------------------------------+
Phone: plugin navigation > Coordinator
+------------------------------+
| < Product [Delivery lead v] : |
| [Chat] [Tasks] [Outcomes]     |
+------------------------------+
| Chat messages                |
| Proposal: Review fix         |
| [Inspect] [Approve]          |
| Input queued: 1 [Cancel]     |
+------------------------------+
| [Message]             [Send] |
+------------------------------+
Phone instance/filter selection: bottom drawer
+------------------------------+
| Choose instance       [Done] |
| (o) Delivery lead            |
| ( ) Reviewer                 |
| [Add instance]               |
+------------------------------+
```

Desktop split composition and phone tabs are required; widths and wording are illustrative. Header/composer are pinned, only the active tab body scrolls, and selectors use bottom drawers. Empty: create an instance. Loading: retain the shell. Disconnected: retain draft and retry identity. Paused: show retained input count and Resume. Revoked/unsupported: explain the blocked capability and link to host settings. Pending human interactions use native shared controls.

Applicable criteria: `AC-PLUGINS-COORDINATION-009.1`, `AC-PLUGINS-COORDINATION-009.2`, `AC-PLUGINS-COORDINATION-009.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps && pnpm --filter @kandev/plugin-sdk test && pnpm --filter @kandev/plugin-sdk typecheck)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm exec vitest run lib/plugins/managed-conversation.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/managed-conversation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-managed-conversation.spec.ts)
```

Evidence to create:

- `apps/web/lib/plugins/managed-conversation.test.ts`: `receipt ordering, retry keys and revoked state`.
- `apps/web/e2e/tests/plugins/managed-conversation.spec.ts`: `conversation and lifecycle states`.
- `apps/web/e2e/tests/plugins/mobile-managed-conversation.spec.ts`: `phone tabs, drawer, keyboard and long content`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/web/lib/plugins/host-api.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/registry.ts`
- `apps/packages/plugin-sdk/src/index.ts`
- `apps/web/components/plugins/`
- `apps/backend/cmd/plugin-fixture/`
- `apps/web/src/locales/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 02](task-02-capability-settings.md)
- [Task 03](task-03-managed-lifetime.md)
- [Task 04](task-04-restricted-tools.md)
- [Task 05](task-05-durable-input.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 08](task-08-execution-controls.md)

## Risks

The task-panel facade assumes a task context. Add explicit workspace/conversation context without breaking old consumers or importing coordinator product state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

WO14 is complete. Added the typed SDK and Host APIs, canonical task-status and usage
facades, reusable managed conversation UI, recovery fences, and the host-owned
read-only retained transcript. The packaged fixture now registers a public-API
managed chat route and drives the same components used by plugins. Its source is
`apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`; the package output
is regenerated by `make -C apps/backend e2e-plugin-package`.

The desktop and phone E2E flows passed. They cover instance switching, task and usage
surfaces, message retry identity, pause/resume, disconnected/revoked/unsupported
states, retained transcripts after uninstall, phone tabs, touch drawer selection,
keyboard submission, long transcript content, and horizontal overflow. The drawer
now closes after a phone or desktop selection. The fixture maps the Host's uppercase
command status values so successful admission clears the composer.

Validation passed:

- `pnpm --filter @kandev/plugin-sdk test` and `pnpm --filter @kandev/plugin-sdk typecheck`.
- Six focused web test files: 40 tests passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, and focused ESLint.
- Desktop `managed-conversation.spec.ts` and phone `mobile-managed-conversation.spec.ts`.
- Fixture package build and UI bundle syntax check.
- Public docs tests (62), 47-page validation, catalog/spec lint (306 decisions and
  1,162 specifications), and `git diff --check`.

### Review remediation (2026-09-27)

Composer drafts and retries are scoped to immutable plugin/workspace/instance
identity. Late input and status reads are discarded after an instance switch, and
controller calls remain bound to the originating instance. The managed-chat hook
regression and deferred-response desktop and phone coordinator E2Es passed. The
composer identity deliberately excludes mutable task status so an open composer
does not remount when task projections change.

The PR fixup also restores 28px minimum targets for desktop chat controls while
keeping 44px targets for phone and coarse-pointer use. Desktop and phone managed
conversation E2E cover the respective sizes.
