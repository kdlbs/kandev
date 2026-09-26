# ADR-2026-09-18-plugin-publisher-trust: Registry-attested publisher identity

**Status:** accepted
**Date:** 2026-09-18
**Area:** protocol

## Context

Plugin manifests supply their own author names. The current catalog preserves these names without an ownership check.
Packages with author `kandev` can originate from unrelated repositories.
The user accepted separate author credit, verified publisher identity, reserved Kandev attribution, and digest-bound installation evidence.

## Decision

The official registry establishes repository ownership and binds that identity to the release archive digest.
The host trusts this evidence only through its canonical HTTPS catalog endpoint.
Custom sources and URL overrides do not grant publisher verification.
Only an explicitly approved registry entry with `kdlbs` ownership receives the Official Kandev designation.
Declared author text remains untrusted credit.

The host stores evidence with the installed release. Replacement packages require new evidence.
An administrator can verify an existing native version by comparison with the exact trusted release package.
This action changes attribution only and preserves the original installation source.
Automatic updates cannot change verified publisher identity or reduce verified status.
The [publisher design](../specs/plugins/system-design/publisher-identity.md) defines the contracts and limits.

## Consequences

The design closes self-declared publisher impersonation without a publisher key-management service.
The registry workflow and canonical catalog transport become explicit trust dependencies.
Existing installs and custom-source packages remain usable but unverified.
Verification identifies origin. It neither certifies code safety nor grants permissions.
Enterprise trust roots, offline verification, and revocation require a later decision.

## Alternatives considered

- A reserved-name blocklist alone leaves repository, source, and badge metadata forgeable.
- Trusting every configured catalog lets a custom source mint the Kandev identity.
- Checking a manifest repository URL proves no relationship between that repository and installed bytes.
- Mandatory publisher signatures provide portable evidence but require keys, identity enrollment, rotation, and revocation beyond this scope.
- Treating every official-catalog entry as first-party misattributes curated community releases.
