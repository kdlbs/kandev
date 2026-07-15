---
id: "05-native-notifications"
title: "Native notifications and duplicate suppression"
status: done
wave: 4
depends_on: ["04-updater-runtime-settings"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 05: Native Notifications and Duplicate Suppression

## Acceptance

- Desktop-owned launches emit native notifications for waiting-for-input and session-failure
  events when allowed by existing notification preferences.
- Each event identity produces at most one native notification; the browser Web Notification and
  backend shell-command System provider are suppressed only for the desktop-owned launch.
- Browser, CLI, and service notification behavior remains unchanged.
- Dock reopen and second-instance activation show/unminimize/focus the existing window without
  inferring a notification route.
- Permission denial keeps in-app indication/toasts and does not repeatedly prompt.

## Files Likely Touched

- `apps/desktop/src-tauri/src/main.rs`, `lib.rs`, and a native notification module
- `apps/desktop/src-tauri/Cargo.toml` and capabilities
- `apps/desktop/src-tauri/src/backend.rs` for the desktop-owned launch marker
- `apps/backend/internal/notifications/service/` and provider tests
- `apps/web/lib/ws/handlers/notifications.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/hooks/use-session-failure-toast.ts`
- New desktop notification bridge modules and focused tests

## Verification

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
cd apps/desktop/src-tauri && rtk cargo fmt --check && rtk cargo test --features desktop-runtime
cd apps/web && rtk pnpm test notification
cd apps/web && rtk pnpm run typecheck
```

Tests must cover event identity deduplication, desktop-only provider suppression, both event types,
preference-disabled behavior, focus without inferred routing, and permission denial.

## Platform Limitation

The official Tauri notification plugin can send notifications on macOS, Windows, and Linux, but
its action/click callback API is mobile-only. Dock reopen and second-instance activation therefore
show, unminimize, and focus the existing main window without consuming a task route. The
notification body click itself cannot be reliably correlated with an arbitrary payload on all
three desktop platforms, so generic focus and activation events never navigate.

## Output Contract

Update this task to `done` and check its plan item only after duplicate suppression is proved at
the desktop/backend/browser boundary.
