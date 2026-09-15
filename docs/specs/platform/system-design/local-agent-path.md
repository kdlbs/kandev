---
status: current
system: platform
requirements:
  - REQ-PLATFORM-LOCAL-AGENT-PATH-001
---

# Local agent executable discovery design

## Purpose and boundaries

Normalize the backend child environment at the native launcher's shared boundary.
This follows the existing ownership in
[the native launcher ADR](../../../decisions/2026-08-08-go-launcher-owns-all-launch-modes.md).
It does not change agent identity or make discovery search a different filesystem
from the process that will execute an agent.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LOCAL-AGENT-PATH-001` | Startup environment and consumer flow |

## Startup environment and consumer flow

`internal/launcher/env.go:backendEnvForConfig` constructs the child environment
used by managed run/start and dev startup. Service definitions also invoke the
native launcher. After applying child-specific environment overrides, a small
Unix-only helper appends the effective user's absolute `.local/bin` path unless
already present. Preserve inherited PATH ordering and do not gate addition on
the directory existing: installation may create it after startup.

For system services (`KANDEV_SERVICE_MODE=system`), look up the effective UID
with `os/user.LookupId` and use that account's home. Do not fall back to inherited
HOME when account lookup fails: launchd may leave HOME unset or naming root after
selecting another account through `UserName`. For regular launches and user
services, use the final child HOME when valid; otherwise use `os.UserHomeDir`
when it yields an absolute home. Do not substitute `KANDEV_HOME_DIR`, the database directory,
the workspace, or a privileged installer's home. Missing/unusable home discovery
leaves PATH unchanged. Preserve Windows behavior. Avoid an empty leading PATH
component when the original PATH is empty.

The launched backend performs `agents.CursorACP.IsInstalled` through
`WithCommand("cursor-agent")`, which uses `exec.LookPath`. Local agentctl launch
inherits the backend environment through `environmentWithOverrides(os.Environ(),
...)`; existing per-execution overrides remain authoritative. Verify this
producer-to-consumer relationship with a subprocess regression, not only string
assertions. Restarted backend children must retain the normalized environment.

`controller.SetJobBroadcaster` already invalidates discovery and refreshes host
capabilities after install success. That flow needs no additional PATH mutation.
Keeping `.local/bin` present from startup also covers already-running local
utility processes created by the corrected launcher before installation.

## Service compatibility

`renderLaunchdPlist` currently supplies system directories and detected Node tool
directories. Existing plists still reach the corrected shared launcher boundary,
so the repair must work without regenerating a plist. Keep service account
selection and Node tool PATH precedence intact. The shell's HOME and the Kandev
data directory are different identities.

## Failure and recovery

An absent executable remains unavailable under existing discovery checks.
Authentication and capability-probe failures remain distinct from installation.
Deploying the fix requires starting the corrected runtime; an already-running
old binary does not gain a new environment automatically. Do not terminate
existing task sessions as part of the repair.

## Persistence, security, and observability

No schema, settings, API, or event changes. Do not execute shell startup files
to recover PATH, prepend user directories ahead of configured tools, or log the
full environment. Existing install and capability status events report recovery.
Public CLI documentation should explain the supported local executable directory
after implementation.
