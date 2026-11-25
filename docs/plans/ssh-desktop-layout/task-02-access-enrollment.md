---
id: "02-access-enrollment"
title: "Access and enrollment"
status: done
wave: 2
depends_on: ["01-detail-frame"]
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-004.10
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.15
  - AC-EXECUTORS-SSH-CONNECTIONS-004.16
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
  - AC-EXECUTORS-SSH-CONNECTIONS-004.20
  - AC-EXECUTORS-SSH-CONNECTIONS-004.21
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Access and enrollment

Group access recovery actions and let users compare methods in one row on wide screens.

## Files and dependencies

SSH components live in apps/web/components/settings/ssh-connections/.
connection-access.tsx, connection-trust.tsx, enrollment-methods.tsx, enrollment-panel.tsx and enrollment-review.tsx. Extend e2e/helpers/ssh-enrollment.ts and ssh-access-setup.ts. Review docs/public/executors.md for affected instructions.

Requires the named container from task 01. Preserve request eligibility, explicit confirmation and credential cleanup. Backend protocols are outside scope.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-previews). The settings page owns scrolling.

```text
UI-02: Access after trust
[Test SSH access] [Install public key]
Wide:   Password   | Existing identity | Install manually
        Selected input
        Close setup   [Review installation]
Phone:  Method rows, input, Review installation, Close setup
```

## Acceptance and risks

1. Before trust, show host inspection. After trust, group eligible installation with the login test. A rejected login keeps install and diagnostics beside the failure. A missing public key cannot offer installation.
2. Three method choices share a row when the pane is wide. In a narrow pane they stack. Show only the selected field. Resize while typing and prove one field, stable selection, retained focus and zero requests on resize.
3. Review shows full labelled fingerprints and the target, with Back and Install together. Back clears the password. Verify pending, rejected, unknown and verified states. The only enrollment request follows explicit confirmation.

## Verification

Add a failing browser layout assertion first. Check both sides of the 54rem container threshold and capture 1280, 1440 and 1920px desktop views, phone and coarse tablet. Use disposable fixtures for command checks.

```bash
pnpm --dir apps/web exec vitest run components/settings/ssh-connections/enrollment-experience.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
```

## Results

On the baseline build the 1440px browser showed three method rows. A unit test
also reproduced the lost password draft on resize. On the new build, tests
passed for a single desktop row, one focused credential input, explicit review
and confirmation, and recovery actions beside rejection. The four focused
enrollment and access suites passed 24 tests.

A separate full Chromium session toggled the sidebar at 1150px and applied
actual 200% tab zoom to the same page. The form switched to three rows,
retained the selected password field, its focused draft, and issued zero SSH
commands. The check used disposable fixture data. The main instance and manual
SSH lab were outside this run.

Desktop and phone Playwright checks for this work order passed. The shared
builds, typecheck, configured lint, formatting and i18n checks also passed.
See the [combined results](plan.md#verification-results) for reruns and scope.
