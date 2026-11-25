---
id: "01-trust-recovery"
title: "Recover from blocked host trust"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-004.16
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Recover from blocked host trust

Use the trust operation and error code together in operation-feedback and connection-trust. Keep generic deletion feedback scoped to deletion. Keep both fingerprints, account and hop in the review. Preserve server authorization, route-stamp checks and the in-use guard. Never release a runtime or overwrite its pin as part of recovery.

Test a real in-use conflict on a disposable fixture, then open a fresh draft through the existing New connection navigation. Cancel must leave the original inspectable. Verify ordinary new trust, expired observations and unrelated errors still follow their own paths. Update public executor instructions and localize each new message.

## ASCII UI preview

UI-01 from the [combined preview](plan.md). After blocked confirmation, the operator sees saved and observed keys, the reason the key is protected and [New connection]. Phone places the action immediately below the reason.

## Verification

```bash
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts
pnpm --dir apps/web run i18n:check
```

## Lab verification

Desktop Chromium and an emulated Pixel both tested the original lab connection against the replacement SSH server. The test detected a changed host key. The explicit trust request returned HTTP 409. The review kept the fingerprints visible and offered New connection, which opened an empty draft. The separately verified connection still authenticated.
