---
status: active
system: plugins
created: 2026-10-22
owners:
  - kandev
---

# Theme Packages, Imports, and Marketplace

## Overview

Theme authors distribute data-only theme packs; administrators import them or
install them from the existing marketplace. Users choose installed variants
through [application appearance](../../ui/requirements/application-themes.md).
Plugins owns the artifact identity, validation, inventory, install authority,
release lifecycle and marketplace contribution contract. UI owns applying it.

The user requested a system comparable to plugins, with native imports, existing
VS Code color-theme JSON import, and contributor marketplace publication. No
production Monokai package or external repository is created by this plan.

## Terminology

- **Theme pack:** A versioned data-only package containing one or more named
  light/dark theme variants and attribution metadata.
- **Import:** Validating a local native pack or standalone VS Code theme file,
  showing a report, and installing a reviewed immutable result.
- **Variant:** A stable key and label inside a pack. Labels are not identities.
- **Compatibility report:** Supported fields, approximations, ignored fields,
  missing values, and contrast findings visible before installation.

## Requirements

### REQ-PLUGINS-THEMES-001: Portable data-only contributions

**Intent:** Contributors add themes without shipping executable integrations.

#### Acceptance criteria

- **AC-PLUGINS-THEMES-001.1:** A theme pack shall declare package identity,
  version, author, description, host/schema compatibility, and unique variant
  keys/labels with a light/dark base. It shall express application, editor,
  syntax, semantic and diff colors as validated data with deterministic defaults.
  Adding a conforming pack shall require no theme-specific host changes.
- **AC-PLUGINS-THEMES-001.2:** Theme packs shall require no binary, native UI
  bundle, CSS, script, runtime, permissions, or external assets. Mixed executable
  contributions, arbitrary CSS values/selectors, unsafe paths, links, oversized
  data and incompatible schemas shall be rejected before extraction/activation.
- **AC-PLUGINS-THEMES-001.3:** Native packs shall share the established package
  integrity/checksum, identity/version, installation and management contracts,
  with an explicit data-only profile. A marketplace label shall never authorize
  an executable package as a theme. Existing executable plugins and canvases
  shall retain their existing validation and execution behavior.
- **AC-PLUGINS-THEMES-001.4:** The contributor-facing format, machine-readable
  schema, validator, packaging command, and portable renderer mapping shall be
  documented and verified with a minimal pack containing two distinct variants.
  Authors shall retain ownership and declared license information.

### REQ-PLUGINS-THEMES-002: Reviewed imports

**Intent:** Existing theme files can be brought into Kandev with clear limits.

#### Acceptance criteria

- **AC-PLUGINS-THEMES-002.1:** An instance administrator shall be able to import
  a native theme archive or standalone VS Code color-theme JSON/JSONC file.
  Supported color/token fields shall be converted to the same canonical theme
  data used by native packs. Missing Kandev roles use a declared base, and an
  absent/ambiguous base requires an explicit light/dark choice during review.
- **AC-PLUGINS-THEMES-002.2:** Before confirmation, import shall show identity,
  author/version/license when supplied, variants, scoped visual samples, and a
  compatibility report distinguishing ignored fields from invalid input.
  Unsupported inheritance, executable extensions and external references shall
  be rejected with recovery guidance; import shall never fetch such references.
- **AC-PLUGINS-THEMES-002.3:** Confirmation shall install exactly the reviewed
  bytes/conversion and metadata under existing instance-admin authority. A
  changed input, stale review, conflicting version or lost authorization shall
  require new review or a clear failure without altering the current install.
  Cancel shall leave installed inventory and saved appearance unchanged.
- **AC-PLUGINS-THEMES-002.4:** Identical repeated imports shall not create
  duplicate packs. Distinct files with the same display name shall remain
  separate unless the administrator explicitly reviews a valid package update.
  Unknown source license/author shall not be invented, and an imported theme
  shall not be published or automatically added to any registry.

### REQ-PLUGINS-THEMES-003: Marketplace and package lifecycle

**Intent:** Theme packs are discoverable, publishable, and manageable through
one ecosystem rather than a hardcoded list or a second marketplace.

#### Acceptance criteria

- **AC-PLUGINS-THEMES-003.1:** The existing marketplace shall expose Themes
  with search, source attribution, package/variant details, compatibility and
  installed/update state. Installation from a listing shall enter the same
  data-only review/validation path as native import and verify curated identity,
  version and artifact digest. Older catalogs without themes remain usable.
- **AC-PLUGINS-THEMES-003.2:** Contributors shall be able to publish a native
  theme release from their own repository and request curated marketplace
  inclusion through the existing registry pull-request process. Registry CI shall
  validate the actual release artifact with the host's data-only validator,
  require redistribution license/attribution, and never execute package content.
- **AC-PLUGINS-THEMES-003.3:** Install/update/enable/disable/uninstall and restart
  shall preserve package identity and valid current content through the existing
  lifecycle. A failed update shall preserve the previous usable release. No
  theme lifecycle path shall spawn a plugin runtime, load a JS bundle, issue
  host credentials, or bind workspace capabilities. Successful inventory changes
  shall be observable by connected clients; reconnect/refresh reconciles missed changes.
- **AC-PLUGINS-THEMES-003.4:** An unavailable selected variant shall become
  ineligible without deleting a user's saved reference. Built-in recovery shall
  remain available. Theme packages are instance-wide, while choosing one remains
  browser-local; non-admin users may choose installed themes and browse listings
  but cannot mutate the shared inventory or marketplace-source configuration.
- **AC-PLUGINS-THEMES-003.5:** Phone users shall browse details and inspect
  import compatibility before confirming through a focused contained surface.
  Failure, cancel and back shall preserve installed/selection state; long reports
  scroll internally and primary actions remain touch-reachable and safe-area clear.

## Out of scope

Running VS Code extensions, VSIX/marketplace scraping, automatic remote include
resolution, fonts/icons/layout changes, executable plugins that also register
themes, user-created marketplace accounts, automatic repository/release/PR
publication, paid themes, remote licensing services, personal install libraries,
and publishing the sample pack as part of host implementation.

## Related contracts

This is the data-only theme exception to the current managed-binary installation
profile in [Plugins](plugins.md). Canvas distribution retains its own independent
static-web-app profile. Appearance remains UI-owned.

## Implementation plan

- [Extensible theme support](../../../plans/application-themes/plan.md)
