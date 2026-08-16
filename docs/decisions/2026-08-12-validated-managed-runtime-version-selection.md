# ADR-2026-08-12-validated-managed-runtime-version-selection: Validate and Persist Managed Runtime Version Selection

**Status:** accepted (amended 2026-08-16)
**Date:** 2026-08-12
**Area:** backend, frontend, protocol
**Supersedes:**
[ADR-2026-07-26-user-managed-agent-runtime-updates](2026-07-26-user-managed-agent-runtime-updates.md)

## Context

Kandev originally invoked managed npm ACP runtimes by an unversioned package
name. The Settings action could prepare the current npm `latest` target, but it
could not select an older version. This made recovery impossible when an npm
release was only partly published, platform artifacts were missing, or the
latest runtime failed ACP initialization. Clearing the matching `_npx` tree did
not help because the next unversioned invocation selected the same broken
release.

The recovery surface must not turn into an arbitrary package installer. Package
identity, registry location, and ACP arguments are trusted Kandev metadata.
Users need to choose among valid versions of that package, validate the
candidate, and keep a known-good version across Kandev restarts.

## Decision

Kandev lets an operator select an exact stable version for a built-in managed
npm runtime. The backend obtains the version catalogue from npm metadata,
validates strict stable SemVer values, and accepts only a version published for
the trusted package. Callers cannot provide a package name, registry URL, tag,
prerelease, command argument, or shell text.

Kandev prepares the candidate under its exact `package@version` npm execution
key and probes it through ACP before activation. Candidate preparation or probe
failure leaves the existing active selection and capability catalogue
unchanged. Kandev persists the trusted package identity and candidate as the
install-wide active version only after a successful ACP probe, then publishes
the candidate capabilities.
This ordering makes the persisted active version the last known good version;
automatic silent rollback is unnecessary.

Future standalone host probes, utility calls, and local agent sessions resolve
the persisted exact version. Existing sessions keep their current process.
SSH, container, and other remote runtimes do not inherit the host selection and
continue to resolve their own runtime. When no selection record exists, Kandev
retains the legacy unversioned host behavior until the first candidate is
successfully activated.

The managed package and exact-version override belong only to an agent's ACP
command surfaces. They do not replace `PassthroughConfig.PassthroughCmd`, an
interactive authentication helper, or either surface's install recipe. Agents
such as Pi can therefore use an ACP adapter package for structured execution
and a separately distributed native CLI for terminal passthrough.

The selection is install-wide and per built-in agent. A saved selection applies
only while its stored package identity matches the agent's current trusted
package metadata; a package change starts with no selection and requires fresh
validation. The selection survives backend and browser restarts independently
of npm's best-effort cache. If reading a saved
selection fails, Kandev fails the new host-managed launch or probe visibly
instead of bypassing the selection with an unversioned package. If npm evicts
the selected version's cache, the exact version is prepared again when needed.

## Consequences

Operators can recover from a broken latest release in Settings without shell
access or a Kandev restart. Runtime changes become attributable: after the
first activation, host launches use the selected exact version until another
candidate passes validation.

The backend now owns a small durable selection record and must route it through
every host-local managed-runtime command path. Version catalogue lookup remains
dependent on registry availability, and cached versions are not a Kandev-owned
artifact inventory. A version that has disappeared from the registry can still
fail to reinstall after npm cache eviction.

The ACP probe is the activation boundary, not an assertion that every provider,
model, or future prompt will succeed. Authentication-required or failed probes
do not activate a candidate because Kandev cannot validate and publish its
capabilities.

## Alternatives Considered

- Latest-only repair was rejected because cache replacement selects the same
  broken release and gives the operator no recovery path.
- Free-form package specs or versions were rejected because they would let a
  Settings request execute an arbitrary npm package.
- Automatic fallback after a failed launch was rejected because it hides the
  version actually used and can make separate launches behave differently.
- A Kandev-owned package directory and lockfile were rejected for this
  iteration. Exact npm package specs and version-specific execution keys provide
  transactional selection without reimplementing package storage.
- Applying the host selection to remote executors was rejected because each
  remote runtime has separate platform, cache, credentials, and ownership.
- Reusing the managed ACP package for terminal passthrough was rejected because
  an ACP JSON-RPC stdio adapter is not an interactive PTY application, and some
  integrations distribute those surfaces as different packages and binaries.
