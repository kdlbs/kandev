# 0011: Service-only UI self-update

Status: accepted
Date: 2026-05-29
Area: backend, frontend, cli

## Context

Kandev can be installed through npm/npx or Homebrew and can also run interactively from a shell. Updating a running interactive process from the UI is brittle: the process may not have a stable service manager, restart semantics, or a durable place to hand work off before the backend exits.

The service installer already owns the OS-specific restart contract for systemd and launchd. That makes it the right boundary for UI-driven self-update.

## Decision

UI self-update is supported only when the backend proves all of these are true:

- It is running under `kandev service install`.
- The unit/plist contains kandev's managed marker and `KANDEV_SERVICE_*` environment.
- `<KANDEV_HOME_DIR>/service/install.json` matches the running service manager, mode, install kind, and service file.
- The service is user-mode, not `--system`.
- The install kind is `homebrew`, `npm`, or `npx`.

The backend returns this state from `GET /api/v1/system/updates`. The frontend renders **Apply update** only when the backend reports `apply_supported=true`; otherwise it shows manual update commands.

`POST /api/v1/system/updates/apply` writes an intent file and starts a helper outside the running service's lifetime:

- Linux user services use `systemd-run --user --collect`.
- macOS user agents use `launchctl submit`.
- E2E/dev tests can fake the helper with `KANDEV_E2E_MOCK=true`.

The CLI owns the helper planner through a hidden `kandev service self-update --intent <path>` command. It upgrades the package via Homebrew, npm, or npx, re-runs `kandev service install`, then restarts the service.

## Consequences

- The UI never offers a destructive update button for an unknown install.
- System services remain explicit terminal operations because they need privilege and differ by host policy.
- Tests can cover the backend, CLI planner, and UI gate without mutating a developer machine.
- The service installer must keep metadata and `KANDEV_SERVICE_*` env vars current whenever it rewrites a unit/plist.
