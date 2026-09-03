---
status: draft
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
created: 2026-07-26
updated: 2026-08-22
owners:
  - Kandev
---
# Managed Agent Runtime Versions and Updates System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-RUNTIME-UPDATES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-RUNTIME-UPDATES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

- **GIVEN** OpenCode latest is partly published and its ACP probe fails,
  **WHEN** an operator selects an older published stable version and approves
  **Roll back runtime**, **THEN** Kandev prepares and probes that exact version,
  persists it only after success, and restores its model list without restart.
- **GIVEN** a healthy exact active version, **WHEN** Kandev restarts, **THEN**
  boot probes and later managed commands use the same exact version.
- **GIVEN** an agent has no operator selection, **WHEN** Kandev builds any of
  its managed npm ACP commands, **THEN** the command uses the exact reviewed
  Kandev default and never an unversioned package spec.
- **GIVEN** an agent has a validated operator selection, **WHEN** Kandev builds
  a local, container, or SSH managed npm ACP command, **THEN** the command uses
  that exact selection instead of the Kandev default.
- **GIVEN** an operator selection exists, **WHEN** the operator chooses **Use
  Kandev default**, **THEN** Kandev validates the exact default, deletes the
  selection only after success, and future commands follow shipped defaults.
- **GIVEN** a candidate fails ACP initialization, **WHEN** the job ends,
  **THEN** the previous active version and capabilities remain authoritative.
- **GIVEN** the current version is unknown and the effective version is known,
  **WHEN** the operator selects a published target, **THEN** the UI offers
  **Repair runtime** and validation establishes the selected exact version.
- **GIVEN** effective, current, and target versions match, **WHEN** the dialog
  opens, **THEN** it shows **Up to date** and starts no job.
- **GIVEN** a different target is submitted while a job is active for the same
  agent, **WHEN** the backend receives it, **THEN** it returns the existing job
  and does not run a second candidate concurrently.
- **GIVEN** an agent whose interactive passthrough CLI is separate from its
  managed ACP adapter, **WHEN** the effective ACP version changes, **THEN**
  later passthrough sessions still launch the declared interactive CLI and do
  not launch the ACP package under a PTY.
- **GIVEN** a phone viewport and a long version catalogue or process log,
  **WHEN** the operator selects and activates a version, **THEN** the drawer
  remains contained and the primary action remains touch-reachable.
- **GIVEN** npm reports a newer stable version than the effective version,
  **WHEN** the Agents settings page loads, **THEN** the existing update control
  shows a blue dot and exposes both versions in accessible update information.
- **GIVEN** the update-status lookup fails for one managed package, **WHEN** the
  Agents settings page loads, **THEN** that package shows no blue dot, its
  update control remains usable, and other package statuses still render.
- **GIVEN** npm returns valid stable package metadata in either its object form
  or a supported one-element collection form, **WHEN** the operator opens a
  managed runtime update dialog, **THEN** the dialog lists the stable versions,
  selects npm's stable latest version, and does not show a resolution error.
- **GIVEN** a managed runtime has a long stable version catalogue, **WHEN** the
  operator opens its update dialog, **THEN** the dialog shows the status summary
  and quick choices without rendering the full version history.
- **GIVEN** the operator opens the full version browser and enters a version
  fragment, **WHEN** matching versions exist, **THEN** only matching stable
  versions remain selectable and selecting one previews that exact target.
- **GIVEN** the operator opens the full version browser on a phone, **WHEN** the
  operator searches or selects a version, **THEN** the existing update drawer
  keeps one contained scroll owner, exposes 44px touch rows, and preserves the
  same target selection behavior as desktop.
- **GIVEN** the operator opens a long version catalogue, **WHEN** the operator
  opens the version selector, **THEN** the catalogue appears in an anchored
  popover without increasing the dialog or drawer height, and selecting a
  version closes the popover.
- **GIVEN** a dotted update control on a phone, **WHEN** the operator taps it,
  **THEN** the existing update drawer opens and shows a live authoritative
  preview without requiring hover.
- **GIVEN** a host-local managed runtime exits before ACP initialization with
  npm `ETARGET` and a matching missing dependency version, **WHEN** Kandev
  reads the captured stderr, **THEN** it removes only the trusted package's
  deterministic `_npx` tree and retries the same runtime once with current npm
  metadata.
- **GIVEN** that online retry starts successfully, **WHEN** ACP initialization
  completes, **THEN** the original session continues and no recovery card is
  shown.
- **GIVEN** that online retry fails with the same npm resolution error,
  **WHEN** the failure reaches Kanban or Office chat, **THEN** the UI explains
  the npm cause, keeps sanitized technical details collapsed, and offers only
  **Retry runtime**.
- **GIVEN** a native, SSH, container, passthrough, or unrelated failed launch,
  **WHEN** Kandev classifies the error, **THEN** this automatic npm recovery
  does not run.

## Out of scope

- Automatic runtime installation, automatic operator-selection changes, and
  automatic rollback after launch failure.
- Global npm cache cleanup, registry replacement, dependency substitution, or
  automatic selection of another package version.
- Prerelease, tag, arbitrary package-spec, registry, or shell-command input.
- Kandev-owned npm artifact retention or a package lockfile.
- Removing npm or network access from the launch path, or locking transitive
  dependency ranges inside upstream packages.
- Restarting or hot-swapping active sessions.
- Native-only update channels and separately distributed passthrough or
  authentication packages.
- Persisting job output or reopening the dialog after a browser restart.
