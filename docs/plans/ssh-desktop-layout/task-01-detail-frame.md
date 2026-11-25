---
id: "01-detail-frame"
title: "Connection details and diagnostics"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-004.18
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
  - AC-EXECUTORS-SSH-CONNECTIONS-004.20
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Connection details and diagnostics

Align the saved detail header and active section. Apply the responsive layout to Overview and Diagnostics.

## Files and dependencies

SSH components live in apps/web/components/settings/ssh-connections/.
settings.tsx, manager-header.tsx, connection-overview.tsx and prerequisite-panel.tsx. Extend e2e/helpers/ssh-management-layout.ts and ssh-diagnostics.ts.

This first work order establishes the detail frame. Keep global Settings, collections and the shared Save coordinator outside the change.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-previews). The settings page owns scrolling.

```text
UI-01: Diagnostics
Wide:   Setup checks + result       | Live login + result
Narrow: Setup checks + result
        Live login + result
```

## Acceptance and risks

1. At wide pane widths, endpoint and identity groups share a row on Overview. The header and all three detail panels align with both edges of the connection list, including on ultrawide screens.
2. On Diagnostics, actions and their results occupy separate columns when space permits. The single column puts setup first. A request failure clears obsolete success and cannot update the other operation.
3. Resize with the sidebar open and collapsed and at 200% zoom. Test long names, keyboard order, independent result announcements and the single page scroller. Phone and coarse controls measure at least 44px.

## Verification

Add a failing browser layout assertion first. Check both sides of the 54rem container threshold and capture 1280, 1440 and 1920px desktop views, phone and coarse tablet. Use disposable fixtures for command checks.

```bash
pnpm --dir apps/web exec vitest run components/settings/ssh-connections/prerequisite-panel.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-management-layout.spec.ts tests/settings/ssh-diagnostics.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-management-layout.spec.ts tests/settings/mobile-ssh-diagnostics.spec.ts
```

## Results

The baseline Overview headings were 120.5px apart vertically at 1440px.
A subsequent setup check also cleared the previous live-login failure.
Both assertions passed on the new build. Desktop browser checks covered the
expanded and collapsed sidebar, the 863/864px threshold, long names, keyboard
order, the settings scroll owner and a coarse tablet. The setup report and
login error remained visible together. The focused prerequisite and SSH hook
suites passed 13 tests.

Desktop and phone Playwright checks for this work order passed. The shared
builds, typecheck, configured lint, formatting and i18n checks also passed.
See the [combined results](plan.md#verification-results) for reruns and scope.

The settings list and all three detail panels now share both page gutters.
The former 1152px limit left 416px unused at a 1920px viewport.
Browser measurements on the isolated lab matched the list on Overview, Access
and Diagnostics at 1920px (1568px content), 1389px with the sidebar collapsed
(1301px content), and 390px (358px content). All six focused desktop and tablet
layout tests passed with the list-to-panel alignment assertion.
