# 0018: Runtime settings overrides

**Status:** accepted
**Date:** 2026-06-14
**Area:** backend, frontend, cli

## Context

`profiles.yaml` is the shipped source of runtime defaults for production, dev,
and e2e. It works well for releases and launch profiles, but users still need
an in-app way to opt into install-level feature toggles such as Office mode
without editing environment variables. Some toggles affect startup-time service
construction and route registration, so changing them safely requires a clear
restart model.

## Decision

`profiles.yaml` remains immutable shipped defaults. User changes are persisted
as install-level SQLite overrides and resolved at backend startup using this
precedence:

```text
explicit environment variable > persisted install override > profiles.yaml active profile > Go zero value
```

The backend owns a typed runtime-flag registry with user-facing labels,
descriptions, stability metadata, risk descriptions, env-var names, and restart
requirements. The Feature Toggles UI renders that registry state rather than
hardcoding env-var behavior in React.

V1 exposes Office mode and Debug mode. Office mode is marked experimental.
Debug mode is the user-facing toggle for debug behavior, including agent
message debug logs; agent message logs are not a separate top-level toggle.

V1 toggle changes require restart. Restart is launcher/supervisor mediated:

- In CLI-managed modes (`kandev start`, `kandev run`, `kandev dev`), the Go
  backend requests restart from the Node launcher, and the launcher restarts
  its backend and web children.
- In service-managed modes, restart delegates to systemd or launchd when the
  backend can prove it is safe.
- If no supported supervisor is available, the UI shows manual restart
  instructions.

The Go backend must not directly fork or exec an unmanaged replacement process,
because it does not own the web child process, launcher logs, browser URL, or
process-tree cleanup.

## Consequences

- Self-host deployment env remains authoritative and auditable.
- Users get a visible, reversible in-app override path.
- Startup-gated features keep predictable initialization semantics.
- The UI can distinguish default, override, env-locked, and pending-restart
  states.
- Restart support needs CLI/service coordination rather than a backend-only
  handler.
- Future multi-user support must restrict Feature Toggles, especially Debug
  mode, to admins.

## Alternatives Considered

1. **Edit `profiles.yaml` from the UI.** Rejected. The file is embedded in
   release artifacts and represents shipped defaults, not mutable user state.
2. **Let DB overrides beat environment variables.** Rejected. Deployment env is
   the safest authority for managed/self-hosted installs.
3. **Make the backend spawn a replacement process.** Rejected. The Node launcher
   or OS service manager owns the process tree.
4. **Expose every profile knob.** Rejected for v1. Mock providers and e2e
   tuning are test infrastructure, not user-facing Feature Toggles.
5. **Show agent message logs as a separate toggle.** Rejected. It is a debug
   sub-behavior and would duplicate the Debug mode mental model.
