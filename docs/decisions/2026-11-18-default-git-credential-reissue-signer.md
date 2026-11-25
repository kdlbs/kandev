# ADR-2026-11-18-default-git-credential-reissue-signer: Provide managed Git credential reissue by default

**Status:** proposed
**Date:** 2026-11-18
**Area:** backend, security

## Context

An agent using managed Git credentials can sit idle long enough for its lease to expire. The next Git or `gh` request can recover when the launch provided a reissue capability. Today, a missing operator signing key prevents capability issuance. An overnight gap can therefore interrupt an otherwise healthy GitHub connection until the agent relaunches. [Managed lease recovery evidence](../plans/git-credential-idle-lease-recovery/plan.md#evidence)

## Decision

Enable reissue by default for managed Git credentials. At startup, prefer a nonblank `githubCredentialBroker.reissueSigningKey` with its existing interpretation. Otherwise, derive the signer from the installation's secrets master key using the [fixed key derivation](../specs/platform/system-design/git-credential-lease-reissue.md#signer-selection). Keep the signer in the backend. Successful startup has a signer unless an operator explicitly disables reissue.

Keep live task, session, repository, and provider authorization at redemption and reissue. The executor receives the existing scoped capability at launch. The backend can verify it after restart when the active key stays the same. [Provider-neutral broker](2026-07-31-provider-neutral-git-credential-broker.md)

Provide an optional backend setting, `githubCredentialBroker.disableReissue`, which defaults to `false`. A backend restart with this setting enabled suspends capability issuance and exchange while permitting ordinary lease issuance and redemption.

## Consequences

- Managed launches receive recoverable leases using their existing provider connection. A blank override selects the installation signer. Deployments that want lease-only operation must set `disableReissue`.
- A managed Docker, SSH, Kubernetes, or Sprites executor on a previously lease-only deployment now receives a scoped bearer valid for up to 30 days from issuance. A leaked capability can authorize replacement leases after the original lease's 12-hour idle expiry while the task, session, repository, and provider remain eligible.
- Disabling reissue suspends exchange of existing capabilities. If reissue is enabled again with the same signer, eligible, unexpired capabilities can be used again. Selecting a different signer prevents the previous capabilities from being verified.
- Installation restore needs the same `master.key` to verify outstanding default capabilities and decrypt previously stored secrets. The secrets loader can silently replace a missing, unreadable, or wrong-length key file. If replacement succeeds, existing default capabilities and previously encrypted secrets become unreadable with the replacement key. The signer-source log shows `installation` whether startup loaded or replaced the key.
- A running agent with a lease alone needs a new process to receive a capability. Use **Restart session** in its session menu. If the session is already stopped, use **Resume**. An upgrade or reconnect can adopt the old process with its original lease-only environment.

## Alternatives considered

- Require an operator-supplied signing key. This preserves opt-in issuance and leaves the idle failure on otherwise working installs.
- Enable reissue for every managed launch without an operator off switch. This simplifies configuration and leaves operators without a way to suspend capability issuance and exchange while keeping managed leases.
- Gate default reissue behind a rollout flag that starts off. This limits initial exposure and requires a separate rollout to fix the idle failure. The durable backend setting lets operators suspend reissue on an installation without adding a user setup step.
- Generate a key at each backend boot. This recovers idle leases within one backend lifetime and invalidates existing capabilities on every restart.
- Store a second random key. This gives independent key rotation and introduces another secret to provision, back up, and restore.
- Extend the lease lifetime. This delays idle expiry and leaves recovery after an empty lease map unsolved.
