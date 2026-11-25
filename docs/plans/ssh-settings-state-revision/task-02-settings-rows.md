---
id: "02-settings-rows"
title: "Keep SSH operations in their Settings rows"
status: done
wave: 2
depends_on: ["01-trust-recovery"]
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-004.15
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.18
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
  - AC-EXECUTORS-SSH-CONNECTIONS-004.20
  - AC-EXECUTORS-SSH-CONNECTIONS-004.21
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Keep SSH operations in their Settings rows

Reuse SettingsGroup and SettingsRow from Editors and the header action pattern from Kubernetes diagnostics. Replace the Access progress strip and duplicate status announcements with compact prerequisite rows and one expanded task. Use equivalent surfaces for setup checks and live login. Keep the existing full-width gutters, responsive peer columns and readable text measure.

In connection-access, enrollment-panel, prerequisite-panel and access-primitives, preserve field identity while resizing. Change the row body on deliberate navigation. Keep its action visible while pending. Clear secrets on exit and preserve one-submit behavior. Use the existing focus rules for Next and Back. Announce an asynchronous result once and leave keyboard focus in place.

## ASCII UI preview

UI-02 from the [combined preview](plan.md). Desktop shows Server trust | Trusted | View details, then SSH access | Not checked | [Test SSH access]. Phone leads with SSH access and stacks the controls. Error and success occupy the initiating section. Confirm the rendered flow in light, dark and reduced motion with a long translation and a phone keyboard.

## Verification

```bash
pnpm --dir apps/web exec vitest run components/settings/ssh-connections
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-management-layout.spec.ts tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-diagnostics.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-management-layout.spec.ts tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-diagnostics.spec.ts
```

## Lab verification

The previous Diagnostics URL selected Access and exposed Check setup. On desktop and phone, local setup and SSH login both succeeded against the disposable fixture. Delete opened the confirmation directly and Cancel kept the saved connection. In a fresh executor editor, Test executor showed the check mark and platform result, and enabled Save. The browser check left the draft unsaved.
