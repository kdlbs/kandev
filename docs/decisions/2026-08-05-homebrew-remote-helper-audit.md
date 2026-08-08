# ADR-2026-08-05-homebrew-remote-helper-audit: Preserve Remote Helpers in Homebrew Installs

**Status:** accepted
**Date:** 2026-08-05
**Area:** infra, workflow

## Context

Kandev release bundles contain the host launcher and `agentctl` plus four platform-specific
`agentctl` helpers used by Docker and SSH executors. Homebrew audits every binary installed in a
formula prefix and rejects helpers whose CPU architecture differs from the installation host.
Removing those helpers would make a Homebrew installation pass audit while silently breaking
remote execution on the omitted platforms.

## Decision

The `kdlbs/homebrew-kandev` tap preserves the complete validated release bundle. The tap owns an
`audit_exceptions/mismatched_binary_allowlist.json` entry for `kandev` that names only these paths:

- `libexec/bin/agentctl-darwin-amd64`
- `libexec/bin/agentctl-darwin-arm64`
- `libexec/bin/agentctl-linux-amd64`
- `libexec/bin/agentctl-linux-arm64`

The exception is not a wildcard and does not cover the host `kandev` or `agentctl` binaries. Tap CI
must continue running Homebrew's formula audit on macOS and Linux so path or bundle changes fail
before release publication.

## Consequences

Homebrew installations retain Docker and SSH support across the release bundle's supported remote
platforms. The tap uses Homebrew's supported, path-scoped audit mechanism for intentional foreign
binaries. Any helper rename, relocation, addition, or removal requires coordinated updates to the
release-bundle contract, the tap allowlist, and tap CI.

## Alternatives Considered

- Prune helpers that do not match the installation host. Rejected because remote targets need not
  match the host and Docker commonly needs a Linux helper on macOS.
- Compress helpers and materialize them at runtime. Rejected because it adds mutable runtime-cache
  behavior and failure modes solely to satisfy an audit that already supports scoped exceptions.
- Download helpers lazily. Rejected because it reintroduces runtime self-downloads and makes an
  installed package incomplete offline.
- Split helpers into separate formulae. Rejected because it complicates installation and release
  synchronization without improving runtime behavior.
