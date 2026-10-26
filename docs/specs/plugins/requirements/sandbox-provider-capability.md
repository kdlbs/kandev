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
- **AC-PLUGINS-SANDBOX-001.6:** Providers shall expose bounded typed capabilities, effective grants, and supported runtime evidence without provider-specific host UI.
  Availability shall not assert active confinement. Unsupported evidence shall remain unknown.
  A second fixture provider shall use the same settings, details, and status contracts without host changes keyed to its identity.

### REQ-PLUGINS-SANDBOX-002: Per-host capability declaration

**Intent:** A provider states what it enforces on each host, so macOS support never inherits Linux claims.

- **AC-PLUGINS-SANDBOX-002.1:** A provider shall answer `DescribeSandboxProvider` for the host OS and architecture that core supplies, and shall return the typed error `host_unsupported` for a host it does not support, which core shall treat, when the core host is darwin, as a refusal before launch with class `sandbox_host_unsupported`; on every other core host the answer keeps its existing handling and no class is added. The error is the gRPC status `FailedPrecondition` whose message is exactly `host_unsupported`; any other error is a provider failure, never a host refusal.
  The Nono provider shall support linux/amd64 (unchanged) and darwin/arm64. Every other pair, including darwin/amd64 and linux/arm64, shall return `host_unsupported`.
- **AC-PLUGINS-SANDBOX-002.2:** The description for a host shall declare its status-evidence schemas and limitations. Core shall treat anything undeclared as unsupported and shall not infer it from another host. Approval support on darwin is a core-owned, host-keyed rule (AC-AGENTS-SANDBOX-011.6) and is not provider-declared; no approval-kind field is added, and the existing approval wire and its Linux validation are unchanged. Process ownership is core-owned and is not provider-declared.
- **AC-PLUGINS-SANDBOX-002.3:** On darwin the Nono provider shall declare no approval wire and shall compile neutral grants to a darwin profile that grants no system temporary directories and no keychain paths and configures no approval webhook; core does not read its denial report for a darwin attempt (AC-AGENTS-SANDBOX-011.7). For identical effective grants and identical per-attempt paths the compiled artifact bytes shall be identical, with grants emitted in sorted canonical path order. The compiled artifact is not claimed to be identical across attempts, because it contains per-attempt paths.
- **AC-PLUGINS-SANDBOX-002.4:** The darwin runtime shall be a packaged artifact in the reviewed plugin package, identified by relative path and SHA-256 in the darwin host's description. Core verifies it against the package checksum entry and copies it into the attempt directory, exactly as for Linux. A mismatch shall refuse the host without fallback to another binary or a `PATH` lookup. The provider never downloads or locates a runtime.
- **AC-PLUGINS-SANDBOX-002.5:** The darwin provider release shall build in the dedicated provider repository and include a darwin-gated test that runs the provider-compiled profile against the canaries of AC-AGENTS-SANDBOX-011.11 with their controls. It is the release gate for the artifact. The Kandev-side suite proves core integration through a real launch. A non-darwin run reports the test as skipped, never passed.

## Exclusions

The remote executor provider interface does not own this capability.
Plugin capability declarations do not isolate trusted installed code from the host operating system.

## Implementation plans

- [Nono sandbox integration](../../../plans/nono-sandbox-integration/plan.md)
