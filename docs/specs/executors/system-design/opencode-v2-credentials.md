---
status: current
system: executors
created: 2026-10-22
owners:
  - tbd
requirements:
  - REQ-EXECUTORS-OPENCODE-AUTH-001
---

# OpenCode V2 Credential Provisioning Design

## Ownership and selection

The lifecycle package owns provisioning between agentctl configuration and ACP
startup. Both normal launch and Kubernetes controller refresh use this boundary.
The stable method ID remains `agent:opencode-acp:files:0`. For V2 the agent
descriptor marks it for launch provisioning and supplies no legacy source files.
Catalog availability means provisioning can be attempted; accounts are checked
at launch rather than exporting secrets during settings polling.

Remote executors honor `remote_credentials`; local Docker retains automatic
credential seeding unless the metadata explicitly supplies a selection.
Standalone uses its existing host authentication. V1 retains `auth.json` copying.

## Export and import

Carry an immutable typed runtime selection from
`resolveOpenCodeCommandOptions` through `AgentExecution`, configure requests,
and worker configuration: family, source, and exact version. Build credential
commands with the managed-runtime recipe, not by searching ACP argv. Validate
selection at both controller and worker. Native/custom runtime sources are
explicitly unsupported for selected V2 bootstrap in this iteration. Use the
structured `launchwrapper.Spec` already carried by upstream; supported wrappers
retain their existing semantics, and unsupported shapes fail before export.
The request never supplies an arbitrary executable or command.

Supported provisioning versions start at 2.1.2. Keep that floor specific to
credential operations and do not lower a newer managed default. Export runs
the resolved package on the controller with `auth export --standalone`.
Construct `cmd.Env` from an explicit controller-side snapshot, including HOME
and XDG overrides. Never merge destination path overrides into that source.
Inspection and import use ACP's final session environment and wrapper.
Source/destination/ambient-store sentinel tests establish path isolation.
Export has a two-minute deadline, a 1 MiB output cap, and platform-owned
process-tree cleanup.

The authenticated `POST /api/v1/agent/opencode/credentials` endpoint accepts the
export array as its request body. It uses existing agentctl auth and instance
middleware. It rejects empty, malformed, or oversized payloads. The configured
structured launcher and exact runtime pin are retained with
`auth import --standalone`. The owned subprocess receives the
configured session environment and credentials on stdin; neither output stream
is exposed to callers. The response acknowledges success explicitly.

The manager excludes configuration, ACP and passthrough startup, and another
import while provisioning is active. Cancellation uses owned-process teardown.
Import uses the CLI merge contract: an existing credential ID is skipped, keeping
refreshed values. Additional accounts for an existing integration remain inactive,
preserving destination account selection. Kandev does not copy SQLite databases.

## Failure and recovery

Advertise a proposed `opencode-credential-bootstrap.v1` capability in
`api.SurvivalCapabilities`, exposed by existing control and instance `/identity`
handlers. Follow `Client.checkLaunchWrapperCapability` for bounded discovery.
Missing capability, malformed replies, and unavailable peers prevent source
export with a precise worker-upgrade or connectivity error. Do not add this
capability to the global survival requirements: only selected V2 provisioning
requires it.

Add an authenticated credential-status operation beside the import endpoint.
It runs bounded `auth export --standalone` locally in the destination, parses
the result in memory, and returns only versioned bootstrap state, never account
values or raw CLI output. This uses a supported CLI rather than SQLite or
human-readable list parsing. A successful empty array means empty; command,
parse, timeout, and size failures mean unknown and deny admission.

Destination state is `empty`, `present`, or `pending`. `present` means saved
accounts exist, not that a provider has validated them. A pre-existing store
without a pending import is destination-owned: skip controller export, even
when its account is revoked or belongs to another provider. Authentication
errors then use the explicit recovery policy below, not automatic replacement.

Before mutating an empty store, atomically persist a secret-free pending marker
in the session's private state, bound to the effective credential-store paths
and runtime family. This is needed because the CLI imports entries separately.
Clear the marker only after successful CLI completion; a lost HTTP response
after completion is safe because inspection sees a present store. A killed
worker or partial import leaves `pending`, requiring a merge retry before ACP.
If a retry sees all IDs already imported, CLI success still completes bootstrap.
Keep the marker through failures; failure to write or clear it denies admission.
Store-path changes invalidate readiness, not credentials. Never use a marker
alone as evidence of authentication or read private provider databases.

Status and import use the same process admission guard as configuration/start.
Reinspect under the import guard; if another caller has completed bootstrap,
do not mutate the now-present store. A pending retry preserves destination
credentials and selection through the CLI's existing-ID merge semantics.
Pending bootstrap still needs the controller source; a completed bootstrap
does not. No automatic deletion, logout, or active-account switch is performed.

Recovery is session-local: stop ACP, use the pinned CLI with the exact session
HOME/XDG environment to inspect accounts, log in again or explicitly select an
account, and resume. Use supported `auth logout` only for an operator-selected
revoked account when required; never reset the whole HOME or conversation store.
Changing the controller account applies to future empty stores only. An operator
recovering an interrupted bootstrap must first retry it successfully; manual
marker deletion is not the documented recovery mechanism.

This does not restore a missing Kubernetes bootstrap environment or provider
history. See the [bootstrap ownership decision](../../../decisions/2026-10-30-opencode-bootstrap-ownership.md).

## Requirement mapping

`AC-EXECUTORS-OPENCODE-AUTH-001.1` and `.4` map to ownership and selection;
`.2`, `.5`, `.6`, and `.10` map to export and import; `.3`, `.7`, `.8`, `.9`,
and `.11` map to failure and recovery.

## Evidence boundary

Real-CLI interruption tests must prove partial writes and lost acknowledgements
without synthetic results being labelled OAuth refresh evidence. Live evidence
must exercise the controller and two independent executor HOMEs across actual
provider expiry and overlapping refresh attempts, then bootstrap a third fresh
HOME and verify all four can still make provider-backed requests. Use the
operator-authorized current lab account and isolate test sessions; publish only
redacted outcomes. If rotation invalidates copies, stop and report the result
before designing a broker or changing the agreed ownership policy.
