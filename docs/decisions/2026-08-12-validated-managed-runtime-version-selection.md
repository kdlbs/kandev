# ADR-2026-08-12-validated-managed-runtime-version-selection: Validate and Persist Managed Runtime Version Selection

**Status:** accepted (amended 2026-08-21)
**Date:** 2026-08-12
**Area:** backend, frontend, protocol, workflow
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

Persisting a selection only after a manual update still leaves every fresh
installation and every unselected agent on an unversioned `npx <package>`
command. Removing npm from the launch path would require Kandev-owned or
user-installed runtime artifacts and is not a requirement for this iteration.
The immediate invariant is narrower: every managed npm ACP command names an
exact top-level package version, while npm remains responsible for cache and
network behavior.

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

Each trusted managed package has an exact default version shipped in Kandev.
Command resolution uses the persisted operator selection when its package
identity still matches; otherwise it uses the shipped default. Kandev does not
copy the default into the database. An installation with no explicit selection
therefore follows newer defaults when Kandev upgrades, while an operator
selection remains stable until the operator changes or clears it.

The resulting effective version applies to every Kandev-built ACP command for
that managed package, including capability probes, utility prompts, local
sessions, containers, and SSH executors. Existing sessions keep their current
process. Native binaries, passthrough commands, and separately distributed
authentication helpers remain outside this package selection.

The managed package and exact-version override belong only to an agent's ACP
command surfaces. They do not replace `PassthroughConfig.PassthroughCmd`, an
interactive authentication helper, or either surface's install recipe. Agents
such as Pi can therefore use an ACP adapter package for structured execution
and a separately distributed native CLI for terminal passthrough.

The selection is install-wide and per built-in agent. A saved selection applies
only while its stored package identity matches the agent's current trusted
package metadata; a package change falls back to the replacement package's
shipped default. The selection survives backend and browser restarts
independently of npm's best-effort cache. If reading a saved selection fails,
Kandev fails the new managed launch or probe visibly instead of bypassing the
selection. If npm evicts the effective version's cache, npm may prepare that
same exact version again when needed.

Settings checks the registry for each managed package when the Agents page asks
for update status. The backend caches successful results for a bounded period,
returns an explicit unknown state when the registry is unavailable, and never
updates a runtime from this read-only check. The UI marks an available newer
stable version on the existing update control. Opening the control performs the
authoritative preview and retains the validated prepare, probe, and activation
flow.

Normal launches retain `--prefer-offline` because an exact package version
removes version-tag drift while preserving npm cache reuse. Explicit previews,
manual updates, and the bounded stale-metadata retry use online-preferred
metadata. Kandev accepts that a cold or incomplete npm cache can still place the
registry on the launch path.

A weekly repository workflow compares the shipped defaults with each trusted
package's stable npm `latest` tag and opens a reviewable PR for changes. It does
not merge automatically. The same centralized version catalogue drives command
construction, tests, and the workflow so runtime pins cannot drift across
separate constants.

## Consequences

Operators can recover from a broken latest release in Settings without shell
access or a Kandev restart. Runtime changes are attributable: every managed npm
launch uses either a reviewed Kandev default or an explicit validated operator
selection.

The backend now owns a small durable selection record and must route it through
every managed-runtime command path. It also owns a bounded, process-local update
status cache. Version catalogue lookup remains dependent on registry
availability, and cached versions are not a Kandev-owned artifact inventory. A
version that has disappeared from the registry can still fail to reinstall
after npm cache eviction.

An exact top-level package pin does not lock transitive dependency ranges. npm
process startup, cold-cache downloads, registry failures, and cache eviction
remain accepted costs. The weekly PR requires maintenance and compatibility
review, while the Settings override lets operators move ahead of a Kandev
release when necessary.

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
- Persisting the shipped default as an operator selection was rejected because
  it would prevent existing unmodified installations from inheriting a newer
  reviewed default in a later Kandev release.
- Removing `--prefer-offline` was rejected because exact package specs already
  prevent top-level version drift, while online-preferred metadata on every
  launch adds registry checks and latency. Explicit update paths remain the
  freshness boundary.
- Automatically installing npm `latest` after an update check was rejected.
  The checker is informational, and every version change remains an explicit
  operator action or a reviewed Kandev pin PR.
- A Kandev-owned package directory and lockfile were rejected for this
  iteration. Exact npm package specs and version-specific execution keys provide
  transactional selection without reimplementing package storage.
- User-installed bridge discovery was rejected for this iteration because it
  would require path ownership, compatibility discovery, and a separate update
  contract. npm remains the supported bridge distribution mechanism.
- Reusing the managed ACP package for terminal passthrough was rejected because
  an ACP JSON-RPC stdio adapter is not an interactive PTY application, and some
  integrations distribute those surfaces as different packages and binaries.
