---
status: draft
system: plugins
created: 2026-10-22
owners:
  - kandev
---

# Sandbox provider capability requirements

## Overview

Plugins owns the optional extension contract, compatibility, and installation lifecycle.
[Agents](../../agents/requirements/sandbox-policies.md) owns policy selection, authorization, and execution outcomes.
Production provider code belongs in a dedicated repository.

### REQ-PLUGINS-SANDBOX-001: Versioned optional provider

**Intent:** Providers can describe sandbox behavior without replacing core process ownership.

- **AC-PLUGINS-SANDBOX-001.1:** Host and provider shall negotiate an explicit major version and supported operations before use.
  Unsupported versions and malformed responses shall fail without fallback.
- **AC-PLUGINS-SANDBOX-001.2:** Adding this optional capability shall preserve plugins that do not implement it and core prefixes with no plugin selected.
- **AC-PLUGINS-SANDBOX-001.3:** Provider calls shall carry host-derived execution identity, bounded inputs, cancellation, and deadlines.
  A provider shall not replace the admitted agent command or confer operator authority.
- **AC-PLUGINS-SANDBOX-001.4:** Plugin crash, upgrade, disable, and removal shall block new dependent launches.
  Existing attempts shall retain enforced policy and core-owned cancellation, with explicit unavailable-service status.
- **AC-PLUGINS-SANDBOX-001.5:** Nono production artifacts shall build and release from a separate repository.
  The Kandev repository shall contain only host, SDK, protocol, UI, and test-fixture changes.

## Exclusions

The remote executor provider interface does not own this capability.
Plugin capability declarations do not isolate trusted installed code from the host operating system.

## Implementation plans

- [Nono sandbox integration](../../../plans/nono-sandbox-integration/plan.md)
