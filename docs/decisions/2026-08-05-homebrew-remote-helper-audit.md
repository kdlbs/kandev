# ADR-2026-08-05-homebrew-remote-helper-audit: Preserve Remote Helpers in Homebrew Installs

**Status:** superseded
**Date:** 2026-08-05
**Area:** infra, workflow
**Superseded by:** [ADR-2026-09-23-compact-runtime-and-remote-helper-assets](2026-09-23-compact-runtime-and-remote-helper-assets.md)

## Context

Kandev release bundles contain the host launcher and `agentctl` plus four platform-specific
`agentctl` helpers used by Docker and SSH executors. Homebrew audits every binary installed in a
formula prefix and rejects helpers whose CPU architecture differs from the installation host.
Removing those helpers would make a Homebrew installation pass audit while silently breaking
remote execution on the omitted platforms.

## Decision

Until the compact runtime distribution cutover, the `kdlbs/homebrew-kandev` tap preserved the complete validated release bundle. The tap owned an
`audit_exceptions/mismatched_binary_allowlist.json` entry for `kandev` that names only these paths:

- `libexec/bin/agentctl-darwin-amd64`
- `libexec/bin/agentctl-darwin-arm64`
- `libexec/bin/agentctl-linux-amd64`
- `libexec/bin/agentctl-linux-arm64`

The exception is not a wildcard and does not cover the host `kandev` or `agentctl` binaries. Tap CI
must continue running Homebrew's formula audit on macOS and Linux so path or bundle changes fail
before release publication.

## Consequences

This decision describes the historical complete-bundle behavior before the compact runtime
distribution cutover. Stable tap installations now use the standard bundle and fetch a verified
remote helper when needed. The tap's mismatched-binary audit exception must be removed with its
first compact formula.

## Alternatives Considered

- Prune helpers that do not match the installation host. Rejected because remote targets need not
  match the host and Docker commonly needs a Linux helper on macOS.
- Compress helpers and materialize them at runtime. Rejected because it adds mutable runtime-cache
  behavior and failure modes solely to satisfy an audit that already supports scoped exceptions.
- Download helpers lazily. Rejected because it reintroduces runtime self-downloads and makes an
  installed package incomplete offline.
- Split helpers into separate formulae. Rejected because it complicates installation and release
  synchronization without improving runtime behavior.
