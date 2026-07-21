# Restart supervisor owns backend restarts

## Status

accepted

## Context

Runtime settings such as Feature Toggles can change values that are only read
during backend startup. The UI therefore needs a restart path that can apply
saved overrides without asking users to manually find and restart the right
process.

The backend process cannot safely replace itself in every launch environment.
Replaying a saved shell command is fragile: shell quoting, aliases, parent
process state, current directory, and secrets are easy to lose or leak. A
detached child spawned by the backend can also leave duplicate or orphaned
Kandev processes.

The existing Settings -> Updates flow provides a safer precedent. It launches
an out-of-process helper through the service manager, then the helper upgrades,
reinstalls, and restarts the managed service. The frontend treats temporary
backend unavailability as expected and waits for a verifiable post-restart
signal.

## Decision

Kandev restarts are owned by the launcher/supervisor that started the backend.
The backend delegates restart requests to that owner through a narrow local
control protocol.

Normal `kandev` CLI launches should run with a restart-capable supervisor by
default. Direct backend/dev/test execution remains supported, but reports
restart as unsupported.

The restart UI must confirm a new backend process is serving requests before
it reports success. A plain health check is not enough because the old process
can remain healthy briefly after accepting a restart. The backend therefore
exposes a per-process `boot_id` and `started_at`; restart polling succeeds only
after the backend is reachable and `boot_id` has changed.

The supervisor stores structured launch data only. It must not persist or
replay a raw shell command string.

## Consequences

- The backend restart endpoint stays small and adapter-driven.
- The primary local restart path works for normal CLI launches without relying
  on systemd, launchd, or container restart policies.
- Service-manager and container restart adapters remain compatibility fallbacks
  for environments that do not use the Kandev supervisor.
- Restart support is conservative: if no restart-capable launcher is present,
  the API returns unsupported with manual guidance.
- The frontend restart progress flow can reuse the self-update pattern, but it
  waits for `boot_id` changes rather than version changes.

## Alternatives Considered

### Store and replay the original run command

Rejected. Shell commands are not a stable or safe launch manifest. They can
lose quoting and aliases, capture secrets, run in the wrong parent context, or
spawn duplicate unmanaged processes.

### Backend spawns a detached replacement process

Rejected. The backend is the process being shut down; it cannot reliably
recreate the parent terminal, service, container, or launch environment that
owns it.

### Only support OS service-manager restart

Rejected as the primary strategy. It is useful for installed services and the
self-update flow, but it does not cover ordinary `kandev` CLI launches.
