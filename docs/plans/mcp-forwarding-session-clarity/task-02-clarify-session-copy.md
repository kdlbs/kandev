---
id: "02-clarify-session-copy"
title: "Clarify session MCP scope timing and save feedback"
status: done
wave: 2
depends_on:
  - "01-contain-proxy-credentials"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.12
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.13
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Clarify Session Customization

## Summary

Keep the composer customization architecture and make its scope, application
state and rejected-save recovery clear on desktop and touch.

## In scope

Session-specific section label, neutral introductory copy, authoritative timing
notice, no primary revision counter, simplified deferred explanation, and
classified forbidden-save copy. Reuse the existing dialog/drawer and picker.

## Out of scope

Inherited overrides, provider reconnect behavior, new overlay/layout architecture,
a new restart action, backend persistence changes, and speculative aesthetic work.

## Acceptance

1. Session choices are labelled Add to this session; inherited origins remain
   named and read-only. Deferred state makes no conflicting idle promise and
   primary copy contains no Desired revision counter. Creation has no unsupported
   application-timing promise.
2. Genuine forbidden save explains that this connection cannot save and previous
   choices remain unchanged. It stays local; shared handled recovery closes the
   overlay without a duplicate error or false pending state.
3. Existing mounted-row/focus/query/polling, connection explorer, closed plug
   markers and phone/tablet geometry continue to pass. All changed copy uses `t`
   and every shipped translation is complete.

## ASCII UI preview

UI-01/UI-02 from [the combined preview](plan.md#ascii-ui-preview):

```text
[Connections] [Customize]
[Applies on next start]
Saved changes apply when the agent starts again.
Inherited MCP servers: <named read-only origins>
Add to this session
[Search...................................]
[ ] Session extras

Forbidden: This connection cannot save MCP selections.
           Your previous choices are unchanged.
```

The desktop dialog and phone full-height drawer keep their current structure.
Touch tablet keeps the bounded drawer. Header/tabs are fixed and Customize owns
one scroller. No nested selection sheet; all semantics/filtering are shared.
AC-006.13 mandates clarity rather than exact sketch phrasing. Use existing UI
primitives, neutral provider-state evidence and 28px/44px control sizing.

## Regression sequence

Add a pure `components/mcp/mcp-selection-feedback.test.ts` regression for the
forbidden classifier, generic save failure, and handled errors; RED must fail
against the current generic presentation. Update existing desktop/phone rejection
helpers to expect the classified message without losing rollback/recovery checks.
Add a controlled deferred-GET journey asserting explicit session label, no idle
promise, and no visible revision counter before changing production copy.
Retain existing real-save, inherited names, pending-refresh and creation journeys.

## Files likely touched

- `apps/web/components/mcp/mcp-selection-picker.tsx`: optional contextual section
  label and classified error presentation; unchanged default for other scopes.
- New `apps/web/components/mcp/mcp-selection-feedback.ts` and test.
- `apps/web/components/task/mcp-session-selector.tsx`: state/body copy and label.
- `apps/web/src/locales/*/settings.json` and generated pseudo/Traditional Chinese.
- `apps/web/e2e/helpers/mcp-selection-feedback.ts` and task MCP desktop/mobile suites.
- `docs/public/agents-and-profiles.md`: accurate timing, forwarding and session instructions.

## Verification

Run from repository root, browser projects sequentially with one worker:

```bash
(cd apps/web && pnpm exec vitest run components/mcp/mcp-selection-feedback.test.ts components/mcp/mcp-origin-labels.test.ts hooks/domains/session/use-task-session-mcp-selection.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-workspace-settings.test.ts components/task/chat/mcp-explorer/mcp-selection-presentation.test.ts components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/mcp/mcp-selection-picker.tsx components/mcp/mcp-selection-feedback.ts components/mcp/mcp-selection-feedback.test.ts components/task/mcp-session-selector.tsx e2e/helpers/mcp-selection-feedback.ts e2e/tests/task/task-mcp-selection.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/task/task-mcp-selection.spec.ts tests/chat/mcp-status.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-task-mcp-selection.spec.ts tests/chat/mobile-mcp-status.spec.ts --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Expand ESLint/test paths for any extra changed files. Rebuild after final
production edits before skip-build. Retain translations unrelated to this change
when generation normalizes older catalog entries. Record actual counts and paths.

After checks, update only the owned :48593 test application with DB backup and
retained data. Prove direct save and state-specific desktop/phone rendering. The
active main-runtime :38383 hop stays unchanged until separately authorized.
Close diagnostic browsers. Do not advertise forwarded-save success without an
owned repaired proxy or an explicitly authorized active-hop upgrade.

## Dependencies

Task 01 for end-to-end forwarding repair. This copy can be tested independently.
Primary session only; sequential, no delegation.

## Risks

The mock provider's deferred state is legitimate. Do not replace it with Applied
or invent reconnection support just to improve the demo. Error classification
must preserve the shared handled-error contract.

## Inputs

Existing composer package and AC-006.9 through 006.13. Screenshot and current
follow-up evidence; existing editor/origin/client/browser regressions.

## Results

Implementation, browser verification and owned deployment complete. Forbidden classifier
regression failed RED against generic presentation; 56 focused tests across
seven files now pass. A real sandbox browser asserted missing session label,
conflicting idle promise and visible revision counter before production copy
changed. Permanent desktop/phone deferred journeys share those assertions.

Typecheck, targeted ESLint, i18n check/ratchet, 62 public-doc validator tests,
47-page public-doc validation, catalog validation and all-spec lint passed.
Traditional Chinese and pseudo generation ran; unrelated catalog values and
prior edits were retained. All seven desktop/tablet journeys passed (2.9m), and all seven phone journeys
passed (1.7m), sequentially with one worker and no retries. The final native
host build, frontend E2E bundle and fixture package were built before the
no-build browser runs. There were no later production changes. The genuine mock-provider deferral is preserved.


The owned :48593 backend and :48595 repaired production-API preview are updated.
Backup: `/tmp/kandev-pr3293-test-runtime/repair-before-deploy.db` (0600).
The existing database and selected IDs were retained; no reseed/reset. Native
binary and web-index hashes match the build manifest. Current main PID 27220
and its start identity remain unchanged. Historical diagnosis PID 3009187 was
already gone when the repair inspected listeners; no main restart was performed.

Fresh deployed browser checked desktop copy, one phone drawer, 44px close target,
no horizontal overflow, readable origins, real forwarded phone save 200,
reload persistence, no local error/reload banner, and baseline restoration 200.
The live state remains authoritative `pending_idle`; no applied/next-start state
was fabricated. Desired revisions increase for the reversible saves/restores;
restoring choices does not roll the revision counter backward. Automated deferred
GET fixtures cover unsupported-provider copy on both desktop and phone.

Evidence: `repair-deployed-browser.log`, `repair-phone.png`, `repair-deploy.log`,
`repair-build/manifest.json` in `/tmp/kandev-pr3293-test-runtime`.
All owned diagnostic browsers are closed. Test backend, MCP fixture and repaired
preview remain running. Stop them with:

```bash
python3 /tmp/kandev-pr3293-test-runtime/stop.py
```

Forward pod/host-local port 48595 to use the repaired preview. Port 48593 is the
underlying direct application. The previous :38383 main-owned hop was not
upgraded or reconfigured. No commit, push or PR mutation was performed.
