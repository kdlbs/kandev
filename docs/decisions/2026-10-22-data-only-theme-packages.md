# ADR-2026-10-22-data-only-theme-packages: Distribute Themes as Data-only Plugin Packages

**Status:** accepted
**Date:** 2026-10-22
**Area:** frontend

## Context

Users want to import themes and contributors want to publish them through the
existing marketplace. A built-in Monokai branch does not provide that ecosystem.
The existing plugin installer assumes a managed executable, while canvases have
a separate static-web-app profile. Themes need colors and syntax styles without
code execution or workspace privileges. Whole-app application also needs a
stable light/dark classification alongside a theme's identity.

## Decision

Add an explicit data-only theme distribution profile to the plugin artifact and
marketplace contracts. The host interprets a versioned allowlist of theme roles;
packages cannot contain runtime, CSS, JS, UI bundles or capability grants.
Native packs and supported standalone VS Code theme imports converge to the same
canonical validated representation. Marketplace curation validates real release
bytes using the host validator, rather than trusting category metadata.

Reuse instance-admin package management and installation-wide inventory. Theme
selection stays browser-local, as the existing color-theme preference is today.
The UI owns generic palette application and keeps binary resolvedTheme for
current consumers; package/variant identity and digest are independent of mode.
No imperative registerTheme API or runtime plugin is required for a theme.

## Consequences

A contributor adds a theme by authoring and releasing data, without changing
host code or running a backend. Themes reuse existing curation, identity,
checksums, install locks and release lifecycle. This adds a deliberate exception
to the managed-binary installer and requires explicit no-runtime handling across
startup, sync, update, enable, disable and uninstall.

VS Code compatibility is a documented subset with conversion reports. Full
TextMate scope fidelity cannot be promised in Monaco's existing Monarch parser
or CodeMirror's Lezer parser. Host-owned default roles cover missing fields.
A saved missing theme falls back visibly without destroying the reference.

## Alternatives considered

- Add named built-in palettes: does not provide imports or contributor publishing.
- Implement themes as executable plugins or arbitrary CSS: grants unnecessary
  execution or presentation authority and complicates reliable recovery.
- Build a separate theme marketplace/install service: duplicates release,
  curation and lifecycle infrastructure already owned by Plugins.
- Use only VS Code data as the native format: cannot state Kandev-specific
  semantic roles and portable renderer behavior without host extensions.
- Require every contributor to port a native format: excludes the requested
  direct import of existing VS Code theme JSON.

## Related specifications

- [Theme packages](../specs/plugins/requirements/theme-packages.md) and
  [design](../specs/plugins/system-design/theme-packages.md).
- [Appearance](../specs/ui/requirements/application-themes.md) and
  [rendering design](../specs/ui/system-design/application-themes.md).
