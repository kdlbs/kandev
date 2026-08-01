# ADR-2026-08-01-standalone-plugin-sdk-module: Publish Plugin Authoring as a Versioned Go Module

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend, protocol, infra, workflow

## Context

Plugin repositories currently import `github.com/kandev/kandev/pkg/pluginsdk`
and use `github.com/kandev/kandev/cmd/plugin-pack`, but both live inside the
backend module at `apps/backend`. Because that module is declared with a
non-fetchable repository path, plugin repositories use a local
`replace github.com/kandev/kandev => ../kandev/apps/backend`, which requires a
sibling Kandev checkout in local development and CI. Kandev multi-repository
tasks do not guarantee that exact directory name, and pull-request package
workflows must fetch the backend merely to build an otherwise independent
plugin.

The SDK is a public compatibility boundary: it owns the Go-native plugin and
Host interfaces, generated `kandev.plugin.v1` transport, manifest contract,
and the package producer whose output the Kandev installer verifies. Those
surfaces need an independently consumable and versioned module even if their
source remains in the Kandev repository.

## Decision

1. Add a top-level Go module at `pluginsdk/` with module path
   `github.com/kdlbs/kandev/pluginsdk`. The SDK remains in the Kandev repository;
   it does not move to a separate repository.
2. The module owns the public Go SDK, generated plugin protobuf/gRPC code, the
   public manifest model and validation rules, the package-writing library,
   and `cmd/plugin-pack`. A plugin must not depend on `apps/backend` to compile,
   test, or package an artifact.
3. The Kandev backend consumes the same module and retains only host-specific
   runtime, installer, persistence, and supervision code. Package-producer and
   installer compatibility is covered by an integration test.
4. The wire contract version (`api_version: 1`) and Go module version are
   independent. Wire changes remain additive within API v1; Go API breaking
   changes follow semantic import versioning.
5. SDK releases use submodule tags such as `pluginsdk/v0.1.0`. Before a stable
   tag is available, plugin migrations pin the exact merged Kandev commit as a
   Go pseudo-version. Production plugin releases may move to an immutable SDK
   tag after it is published.
6. Rollout is staged to avoid an unresolvable self-dependency. The first Kandev
   change adds and verifies the independent module while retaining the existing
   backend-owned copies. After merge, the SDK tag/pseudo-version is resolved;
   a follow-up Kandev change switches the backend to the module and removes the
   old copies. Plugin repositories migrate only after the first change is
   merged.
7. The repository commits a root `go.work` for local cross-module development,
   while each `go.mod` remains independently buildable with `GOWORK=off` against
   published or pseudo-versioned dependencies. The existing backend graph still
   carries the pre-split `google.golang.org/genproto` module through Viper,
   while the SDK's gRPC stack uses split API/RPC modules; the workspace pins
   those two split paths back to the backend's existing monolithic version to
   avoid ambiguous package selection locally. This compatibility bridge is
   workspace-only and does not alter either published `go.mod`.
8. Official plugin PR workflows build a uniquely versioned, all-platform
   package and upload the resulting `.tar.gz` as a GitHub Actions artifact.
   Workflows use `pull_request`, least-privilege read permissions, and never
   auto-install untrusted PR artifacts.

## Consequences

- Plugin-only tasks and CI no longer need a Kandev checkout or layout-specific
  local replacement.
- SDK dependencies become reproducible and reviewable through an exact
  pseudo-version or semantic tag.
- Kandev and plugin repositories can test the same manifest, transport, and
  packaging implementation instead of maintaining author-side copies.
- The SDK module has its own dependency graph, generated-code checks, release
  cadence, and compatibility obligations.
- A short duplication window exists between the initial module merge and the
  Kandev adoption change. Contract parity tests bound that window.
- SDK changes that must be consumed by Kandev or plugins follow the order:
  merge SDK change, publish/resolve its immutable version, then update consumers.

## Alternatives Considered

- **Pin the entire backend module with a remote `replace`.** This removes the
  sibling checkout immediately, but exposes the backend dependency graph and
  gives public authoring APIs no independent version or compatibility policy.
- **Move the SDK to a separate repository.** This produces a simple repository
  shape but adds synchronization and review overhead between the wire contract,
  host implementation, and SDK. A nested module supplies independent Go
  versioning without splitting ownership.
- **Publish only `pkg/pluginsdk`.** Rejected because plugins would still depend
  on the backend for generated protocol code, manifest validation, and
  `plugin-pack`, leaving the plugin-only workflow incomplete.
- **Keep a permanent local `replace`.** This works in a Kandev checkout but
  leaves the backend module non-reproducible outside that layout. A committed
  workspace plus independently resolvable module requirements preserves both
  development and release behavior.
