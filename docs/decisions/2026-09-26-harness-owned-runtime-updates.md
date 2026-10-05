# ADR-2026-09-26-harness-owned-runtime-updates: Delegate Runtime Updates to a Harness That Owns Them

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend, frontend, workflow

## Context

Kandev's managed runtime update system resolves exact versions of a trusted npm
package, stages the candidate under an npm execution key, probes it over ACP,
and persists the selection only after a successful probe
([ADR-2026-08-12](2026-08-12-validated-managed-runtime-version-selection.md)).
That contract assumes the agent's structured runtime is an npm package that
Kandev can install by exact version.

The Oh My Pi harness (`omp`, agent id `omp-acp`) does not fit that assumption:

- Its npm package `@oh-my-pi/pi-coding-agent` ships a Bun bundle. The `omp`
  entry point is `dist/cli.js` with a `#!/usr/bin/env bun` shebang,
  `// @bun` output marker, and `engines: {bun: ">=1.3.14"}`. npm can install or
  link it, but every process it launches needs `bun` on `PATH`, so an
  npm-anchored Kandev update would add a Bun prerequisite to every execution
  environment, including container and Sprite executors that today only need
  the `omp` binary.
- Its canonical installations are not npm-anchored at all: the upstream default
  installer uses Bun when present and otherwise downloads a prebuilt standalone
  binary, and Homebrew, mise, and Nix are supported installation channels. A
  Kandev `npm install -g` update would introduce a second installation next to
  the operator's own and silently change which one runs.
- The harness already ships a package-manager-independent updater,
  `omp update`. It detects the installation method of the resolved `omp`
  launcher (Homebrew, mise, bun, npm, or a standalone binary), installs the
  release for the selected channel, verifies the resulting launcher reports the
  expected version, and repairs a failed package-manager install by replacing
  the launcher with the verified standalone binary. It declines to modify a Nix
  installation.
- That updater always targets its own channel's latest release. It exposes
  `--check`, `--force`, `--plugins`, `--canary`, and `--stable`, and no flag
  that selects an arbitrary published version. Selection, rollback, and staged
  candidates therefore cannot be implemented on top of it.

Operators still need the Settings update surface for such a harness: an
availability indicator, a maintenance job with streamed output, and a
capability re-probe after the change. Leaving these harnesses out of the update
system forces shell access for routine maintenance.

## Decision

Kandev's agent update system has two flavors behind one Settings surface, one
set of endpoints, and one job pipeline:

1. **Pinned runtime.** A built-in managed npm runtime: exact version catalogue,
   staged candidate, ACP probe before activation, persisted selection.
2. **Harness-owned updater.** A built-in agent that declares a trusted
   self-update command. Kandev resolves the upstream stable latest version from
   the trusted package's registry metadata for display and as an advisory status
   reference. If stable latest is newer than the ACP version, Kandev reports an
   update hint. If it is equal to or older than the ACP version, Kandev reports
   unknown because the configured channel is not known. The reference never
   gates approval. Kandev runs the agent's own command on the host, which
   follows the harness's existing channel and may install a different version,
   streams its output, and then probes the agent over ACP. It publishes
   capabilities only after a successful probe that reports a changed version,
   and never persists a version selection.

The harness's own installer is the integrity boundary for a harness-owned
updater. Kandev does not stage, copy, or verify the artifact itself, and it does
not roll back a completed harness update. The harness updater's own version
check, digest verification, and package-manager fallback are the accepted
replacement for Kandev's staged-candidate guarantee.

A harness that declares a self-update command keeps every other surface
unchanged: execution commands, container command construction, install script,
session recovery, passthrough behavior, and the managed npm catalogue. Declaring
the capability is not a claim that the harness is npm-managed.

The update command, the metadata package, and the probe command come only from
built-in agent metadata. A request cannot supply a command, package, registry
location, or version for a harness-owned updater.

## Consequences

Operators get the same Settings update control for a harness-owned updater as
for a pinned runtime, without Kandev assuming an installer for that harness.

Kandev cannot offer version selection, rollback, or candidate validation for
these harnesses, and the UI must not imply that it can. Stable latest is only
an advisory reference; it cannot establish whether a configured canary channel
is current and it never disables the update action. An update follows the
harness's existing channel and may install a different version. The recorded
current version is whatever the post-update ACP probe reports, so it reflects
the harness's channel choice rather than a Kandev-reviewed pin. A successful
updater exit with no ACP-reported version change fails the job and retains the
updater output.

A failed post-update probe fails the job after the harness has already replaced
its installation. Kandev keeps the previous capability catalogue and does not
pretend the version changed, but recovery depends on the harness's own
installer, `--force`, or the operator's package manager.

Two update flavors now exist in one pipeline. Each new built-in agent must
choose exactly one: a pinned npm runtime, or a harness-owned updater. Adding a
third mechanism, such as Kandev staging prebuilt release binaries itself,
requires superseding this decision.

Bun stays a runtime prerequisite only for installations that the harness
updater itself chooses to manage with Bun; Kandev does not introduce it into the
execution environment.

## Alternatives Considered

- **Register `omp` as a pinned managed npm runtime (OpenCode parity).** Rejected
  because it changes the container and remote launch path to an npm execution
  tree that needs Bun at runtime, while `omp update` cannot honor an exact
  version, so the pin, rollback, and repair semantics of that flavor would be
  unreachable or misleading.
- **Give prebuilt release binaries the staging treatment.** A new Kandev-owned
  download, checksum, and platform-matrix subsystem would duplicate verification
  the harness already performs and would add a second update channel that can
  disagree with the operator's Homebrew, mise, or Nix installation.
- **Keep the current behavior and document a manual `omp update`.** Rejected
  because the operator loses the capability re-probe that follows an update, and
  the Agents page would keep advertising models and modes from the previous
  binary.
- **Run `omp update --check` and parse its output for the availability
  indicator.** Rejected because its exit status does not distinguish outcomes,
  and parsing human-readable output is a fragile status contract. Kandev reads
  the package's stable dist-tag directly from the trusted HTTPS npm registry
  endpoint, without requiring the `npm` executable.
- **Let the Settings request name the update command.** Rejected: it would turn
  the update endpoint into an arbitrary command runner.
