# ADR-2026-09-27-opencode-runtime-adoption: Persist OpenCode Adoption Independently of Version Defaults

**Status:** accepted
**Date:** 2026-09-27
**Area:** backend, frontend, protocol
**Amends:** [Default activation](2026-09-07-activate-managed-runtime-defaults.md) and
[validated runtime selection](2026-08-12-validated-managed-runtime-version-selection.md), for OpenCode only.

## Context

OpenCode v2 uses a different npm package. Kandev currently resets a runtime selection when its package changes.
Applying that rule would migrate existing v1 users without their choice.
Native PATH preference could also override an accepted managed v2 installation.
The user requested v2 for new installations, optional migration in Settings, and a durable database choice.

## Decision

Keep one OpenCode agent identity and ACP transport. Persist its adopted distribution and runtime source
independently of ordinary reviewed version defaults. The choice is install-wide across OpenCode profiles.
Existing v1 users retain v1 until they explicitly migrate; fresh installations without legacy evidence default to v2.
The migration selects a Kandev-managed exact package/version and does not replace the independent standalone CLI.
Save the complete selection only after candidate installation and isolated ACP validation succeed.
Managed adoption overrides PATH preference on future Kandev launches.

Default reconciliation may change the version within the adopted family but cannot switch families.
Keep other agents' generation policy unchanged. Resume existing sessions using their native identity and storage.
Do not silently create another conversation on restore failure or promise automatic storage rollback.

## Consequences

The resolver must carry family and source as well as version through all OpenCode launch paths.
Startup imports legacy settings before ordinary reconciliation can erase them.
The update dialog states its shared scope and distinguishes activation from later discovery or resume failure.
The standalone CLI remains under user control. Independent processes sharing OpenCode data remain outside Kandev's process guard.
Implementation details and coverage are in the [design](../specs/agents/system-design/opencode-v2-adoption.md).

## Alternatives Considered

- Replace the package constant and reuse generic reconciliation: silently migrates existing users.
- Store only a migrated Boolean: can disagree with package, version, and native preference.
- Create a second agent identity: fragments profiles and saved session references.
- Replace the global CLI during migration: changes a user-owned installation and depends on its package manager.
- Select versions separately per profile: changes existing runtime scope and permits concurrent mixed-major use of shared data.
