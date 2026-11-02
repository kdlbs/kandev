---
id: "01-prove-plugin-artifact"
title: "Prove and package the OpenCode v2 plugin"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-002
  - REQ-AGENTS-OPENCODE-PLUGIN-004
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-001.2
  - AC-AGENTS-OPENCODE-PLUGIN-001.4
  - AC-AGENTS-OPENCODE-PLUGIN-002.1
  - AC-AGENTS-OPENCODE-PLUGIN-002.3
  - AC-AGENTS-OPENCODE-PLUGIN-004.3
  - AC-AGENTS-OPENCODE-PLUGIN-004.4
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
---

# Task 01: Prove and package the OpenCode v2 plugin

## Summary

Establish the plugin ABI and artifact before integration code relies on them.
Create the private workspace package, deterministic ESM output, and a real
packaged OpenCode 2.2.0 conformance harness.

## Scope and owned files

- Own `apps/packages/opencode-plugin/package.json`, its TypeScript configuration,
  `src/index.ts`, `src/protocol.ts`, core interfaces, and the static registry.
- Own package build/test scripts, `test/registry.test.mjs`,
  `test/conformance.test.mjs`, and a local deterministic model endpoint fixture.
- Define `build`, `typecheck`, `test`, `check:artifact`, and `test:conformance`
  scripts used below. Do not require a paid model or the user's configuration.
- Own the initial Go embed wrapper, asset directory, manifest, and
  `artifact_test.go` in `internal/agentctl/server/opencodeplugin`.
- Own required `apps/pnpm-lock.yaml` updates. Do not add a runtime npm dependency.
- Update this package's `protocol-evidence.md` with actual results and hashes.

Use type-only imports from the pinned v2 SDK. Ship the real v2 plugin shape with
one `kandev` ID and cleanup. Include only a test behavior at this stage. Do not
implement shell outcomes, callback ingress, rollout, or automatic launch here.

## Implementation order

1. Write failing loader/configuration tests against an isolated packaged CLI.
2. Build the smallest actual plugin artifact that passes those tests.
3. Prove a native subscription barrier before a deterministic first job.
4. Prove additive inline JSONC configuration with file/project/managed layers,
   user plugin entries, options, and environment/file substitutions.
5. Prove setup failure, cancellation, repeated setup/cleanup, reserved-ID
   conflict, and an unsupported runtime. Preserve ordinary ACP behavior.
6. Add deterministic asset/digest checks and the source-versus-real-artifact
   evidence record. No fixture can substitute for packaged compatibility proof.

Use fresh temporary HOME and XDG directories only for owned test subprocesses.
Disable auto-update and isolate project configuration. The harness installs only
the exact reviewed CLI into its own cache, or uses its verified cached artifact.
It never touches the user's managed runtime selection or native installation.

## Acceptance

1. The actual OpenCode 2.2.0 package loads the shipped plugin, reaches native
   subscription readiness, emits a native shell result, and calls cleanup.
2. Configuration tests prove preserved user plugin/configuration semantics and
   safe rejection. A test-only second module requires no core rewrite.
3. Regeneration reproduces the checked ESM bytes and manifest hash. A Go-only
   artifact test passes without a JavaScript install during execution.

## Verification

From `apps/`, install dependencies once in a fresh worktree:

```bash
pnpm install --frozen-lockfile
```

After introducing the package and updating its lockfile, run from `apps/`:

```bash
pnpm --filter @kandev/opencode-plugin typecheck
pnpm --filter @kandev/opencode-plugin build
pnpm --filter @kandev/opencode-plugin test
pnpm --filter @kandev/opencode-plugin check:artifact
pnpm --filter @kandev/opencode-plugin test:conformance
```

From `apps/backend/`:

```bash
go test -tags fts5 -count=1 ./internal/agentctl/server/opencodeplugin -run 'TestArtifact'
```

Record the selected CLI version, hash, plugin digest, exact command, and result.
The conformance script must reject an unpinned or incompatible CLI.

## Risks and dependencies

Published SDK resolution and compiled CLI plugin resolution can differ.
The v2 type declarations can hide connection events that the runtime forwards.
Prove readiness at the real stream boundary. Do not use a fixed delay.

If the packaged runtime cannot preserve the required configuration or expose
native events, keep activation blocked and update the design with the evidence.
Do not replace the integration with database or default-server polling.

## Results

Completed on 2026-10-29.

- `@kandev/opencode-plugin` builds one dependency-free ESM entry with esbuild and
  writes the checked artifact plus SHA-256 manifest under
  `internal/agentctl/server/opencodeplugin/assets`.
- The Go embed, manifest, atomic staging, tamper replacement, and dependency
  checks pass with `go test -tags fts5 ./internal/agentctl/server/opencodeplugin`.
- `KANDEV_OPENCODE_CONFORMANCE=1` installed `@opencode/cli@2.2.0`, loaded the
  shipped artifact through inline configuration, retained a file-configured
  user plugin, reached the native subscription barrier, and delivered one shell
  outcome with exit code 3.
- Artifact: 29265 bytes, SHA-256
  `893f9535bd6216d66b2a4535b4d9beafa19351f4d2e945167e168bf25b4f4c30`.
- The packaged command rejects any CLI that does not report `opencode v2.2.0`.
- No runtime npm dependency was added. The artifact is checked in for Go and
  cross-compiled helper builds.
