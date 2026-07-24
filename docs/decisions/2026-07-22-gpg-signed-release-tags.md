# ADR-2026-07-22-gpg-signed-release-tags: GPG-Signed Release Tags

**Status:** accepted
**Date:** 2026-07-22
**Area:** infra, workflow

## Context

Kandev's release workflow creates annotated `vX.Y.Z` tags after merging the
release PR, but those tags do not carry a cryptographic signature. The npm
launcher and all five platform runtime packages already publish SLSA provenance
through npm trusted publishing, so the remaining source-identity gap is the Git
tag used as the release build ref.

## Decision

Normal releases must create GPG-signed annotated tags and verify each signature
in the release runner before pushing the tag. The signing key and optional
passphrase are supplied through a GitHub Environment restricted to `main`.
The environment has no required reviewer, so any maintainer authorized to
dispatch the workflow and access the environment may start a normal release.
The workflow imports the key only
after merging the release PR, requires its full fingerprint to match the
environment's expected fingerprint, and adopts the validated key identity for
the tagger. Missing, invalid, or mismatched signing material fails the release
before tag publication. Historical and backfill tags remain unchanged.

The repository tag ruleset targets `v*`, enables **Restrict updates** and
**Restrict deletions**, leaves **Restrict creations** off, and has an empty
bypass list. Backfill reuses the protected tag and must not weaken that rule.
The pinned key-import action owns key cleanup through its registered post step
on the ephemeral runner.

GitHub and the repository are the source of truth for the public key. The
current key is committed at `.github/release-signing-key.asc`, and its full
fingerprint is recorded with the release configuration so operators can verify
it before running `git tag -v`. The `release-validation` environment remains
an empty default environment, while normal releases use `release`. npm
publication continues to use GitHub OIDC trusted publishing, with contract
coverage preserving provenance for the launcher and every runtime package.

No product spec applies because this is a release-process and supply-chain
invariant rather than a user-invocable product capability.

## Consequences

Future release tags can be tied to a maintained signing identity and checked
against the public key stored in the repository. Normal releases depend on the
`release` environment, the configured expected fingerprint, and valid GPG
signing secrets, while dry runs, desktop validation, and backfills use the empty
`release-validation` environment and do not import the private key. Key
generation, rotation, revocation, GitHub identity association, and environment
configuration remain maintainer operations; public-key distribution is owned
by the repository.

## Alternatives Considered

- Sigstore/gitsign keyless signatures were rejected because they require a
  Sigstore-aware verifier and do not satisfy the established `git tag -v`
  verification workflow.
- Repository access controls alone were rejected because they do not provide a
  cryptographic trust anchor if a write credential is compromised.
- Signing only release commits was rejected because the release tag object can
  still be independently created or replaced.
