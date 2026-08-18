# ADR-2026-08-01-bitbucket-initial-release-remains-unsigned: Bitbucket Initial Release Remains Unsigned

**Status:** accepted
**Date:** 2026-08-01
**Area:** infra, workflow, security

## Context

Kandev currently verifies every plugin package's generated internal
`checksums.txt`, but it does not verify `checksums.txt.sig`; installed packages
are therefore reported as unsigned. The Bitbucket implementation plan had made
a signed release a publication gate even though no host trust policy, verifier,
approved key, or key-rotation process exists. Producing a signature that the
host cannot authenticate would add ceremony without adding enforceable trust.

## Decision

The initial official `kandev-plugin-bitbucket` release may use the current
unsigned marketplace contract. Its package must contain the generated internal
`checksums.txt`, its public GitHub Release may include the standard advisory
release-level checksum asset, and its source remains public and subject to the
official catalog's maintainer review. Kandev will report the installed package
as unsigned, and neither repository may claim that the release is signed or
that its publisher provenance is cryptographically verified.

Signing is not a Bitbucket release gate. Enforceable plugin signing remains
future host-wide work and requires a superseding decision covering signature
verification, trusted-key or identity distribution, rotation and revocation,
and fail-closed policy before official plugins can rely on it.

## Consequences

- Bitbucket follows the trust posture already implemented for other plugins
  instead of inventing an integration-specific signing ceremony.
- Package checksums detect extraction corruption and unexpected file changes,
  but they do not authenticate who produced an archive. Trust still rests on
  the public source and release repository, curated catalog review, and the
  operator's explicit install/update action.
- The signing decision no longer blocks Bitbucket publication; host
  compatibility, live provider behavior, packaged-host, and credential-broker
  evidence remain independent release gates.
- A future signing rollout must deliberately supersede this decision and may
  require republishing or reattesting the plugin.

## Alternatives Considered

- **Block Bitbucket until signing exists.** Rejected because no shipped verifier
  or trust-root policy exists, while the marketplace explicitly supports
  unsigned packages.
- **Generate an ad hoc Ed25519 signature for Bitbucket.** Rejected because the
  host could not authenticate it and no approved distribution, rotation, or
  revocation process exists.
- **Require Sigstore provenance for this plugin alone.** Rejected as a useful
  future direction that first needs a host-wide verification and policy design.
