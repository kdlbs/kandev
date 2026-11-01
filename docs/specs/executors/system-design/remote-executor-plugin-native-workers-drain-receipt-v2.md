---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-010
---

# Remote executor plugins: drain receipt version 2 system design

## Purpose and boundaries

Amends the [drain receipt section](remote-executor-plugin-native-workers.md) of the native worker design for one capability version. The registry, the manifest syntax and the negotiation point stay those of the [versioning design](remote-executor-plugin-capability-versioning.md): `drain_receipt` is registered at versions 1 to 2, the manifest names exactly one, and the profile validation echo must name the same one. The provider contract version stays 1. Names marked **new** do not exist before this change.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-NATIVE-WORKER-010 | Meaning; Preconditions; Outcome table; Envelope fields; Provider obligations; Compatibility; Not proven and risks |

## Meaning

`not_attempted` under version 2 states one host fact: the host never held an agentctl credential for this runtime. Agentctl creates an instance, an agent or a shell only for an authenticated request, so without a credential none of them can have been created through agentctl. It is not a statement about the provider's resource: a job, allocations and worker files can exist, and the provider verifies them itself. The value is reused, not renamed; the capability version decides which meaning a provider may rely on.

## Preconditions

These are stated, not derived, and the rule holds only while they do.

1. Agentctl starts no agent, shell or managed process before an authenticated request **only when a token or bootstrap nonce is configured**. With neither, its auth middleware lets every loopback request through. The host's bootstrap always supplies a nonce, so a host-launched agentctl is covered; an agentctl started by other means is not.
2. The token reference is checkpointed before `CreateInstance` only for a provider that negotiated `durable_bootstrap`, so the rule that a recorded handshake without a recorded credential left no instance requires `durable_bootstrap` (AC 010.4). A provider that does not negotiate it keeps the strict rule for any started handshake.

## Outcome table

Read from the stored record's negotiated set. `credential` is the token revealed from the secret the record references, or the in-memory token of a launch in progress.

| Version | Credential | Handshake started | `credential_recorded` | `handshake_rejected` | `durable_bootstrap` | Outcome |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | none | no | any | any | any | `not_attempted` for `launch_failed`, else `unreachable` (unchanged) |
| 1 | none | yes | any | any | any | `unreachable` (unchanged) |
| 2 | none | no | any | any | any | `not_attempted`, any reason |
| 2 | none | yes | no | no | yes | `not_attempted` |
| 2 | none | yes | yes | any | any | `unreachable` (secret missing or unreadable) |
| 2 | none | yes | any | yes | any | `unreachable` (nonce already spent) |
| 2 | none | yes | any | any | no | `unreachable` |
| any | known | any | any | any | any | drain; `drained`, `unreachable` or `uncertain` as before |

`credential_recorded` is three-valued: absent (the record was written by a host that does not know the field), `false` (written explicitly when a version 2 environment is admitted) and `true`. Only an explicit `false` takes row 4. A record that has a started handshake and no `credential_recorded` stays `unreachable`, and so does one that pins version 1; the host never infers the credential state from other metadata.

## Envelope fields

**New** `credential_recorded` (absent, `false` or `true`) and `handshake_rejected` (`true`, omitted when false) in the `plugin_executor` envelope. A launch that negotiates `drain_receipt` version 2 writes `credential_recorded: false` with its first checkpoint, before any handshake; an environment of version 1 never carries the field until a token reference is recorded. `credential_recorded` is set to `true` in the launch checkpoint that carries the token secret reference, in the launch path and in the durable recovery path, and is never cleared. `handshake_rejected` is set when a handshake returns `agentctl.ErrHandshakeRejected`, in the launch path and in the recovery path, is persisted before the destroy that follows, and is never cleared. Both belong to the `coordination` group of the envelope budget.

## Provider obligations

A provider that negotiated version 2 treats `not_attempted` with a non-empty cleanup claim as the answer to its drain question for every cleanup reason. It still proves, before it reports absence, that the resource is this operation's and stopped, every allocation is terminal, the node contract holds, the worker state is removed and terminal evidence exists. The provider's own earlier abort does not override the host's fact: when the host sends `unreachable` it quarantines.

## Compatibility

Proto and wire are unchanged (the `drain_outcome` value already exists); only its documented meaning is versioned. New host with an old plugin (version 1 declared): unchanged. Old host with a plugin declaring version 2: the install is refused by the capability range check, because the old registry maximum is 1. A manifest cannot offer a range, so a provider ships one version per release: a plugin release that declares version 2 needs a host that carries this registry. Retained records keep their pinned version; a host built before the range cannot attach a record pinned to version 2 and reports the provider unavailable for it.

## Not proven and risks

- Agentctl's only startup process is a sweep for orphaned code-server hosts (`pgrep`, `ps`) that may kill an orphaned code-server host-wide. It runs before any request, so it is not covered by "no process before an authenticated request". It is recorded here as a known hazard and not changed by this design.
- A crash after agentctl accepted a handshake and before the token was persisted leaves `credential_recorded` false: the host never held a usable credential and no instance was created, so the outcome is `not_attempted`. A later recovery attempt repeats the handshake with the spent nonce, agentctl rejects it, `handshake_rejected` is set and the outcome is `unreachable`. Which of the two the provider sees depends on whether a recovery attempt ran first; both are safe.
- Live proof (an agentctl on a real worker, an ssh-reached node) is not part of this change; the tests use fakes.
