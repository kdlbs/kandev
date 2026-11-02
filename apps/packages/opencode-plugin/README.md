# @kandev/opencode-plugin

Private Kandev asset: one OpenCode v2 server plugin supplied automatically to
supported task launches. It is not published to npm and is not a Kandev
marketplace plugin.

## Entry point and lifecycle

`src/index.ts` exports the single `kandev` plugin with an `id` and `setup`.
`setup` reads the private bootstrap file named in `options.bootstrapFile`, waits
for the native event subscription barrier, then calls the private launch host.
Cleanup aborts the subscription, disposes every behavior, and stops the sender
outbox. A failed setup never makes the ACP conversation unusable.

## Behaviors

`src/behaviors/registry.ts` is the static production list. A behavior declares
its ID, version, native event types, `start`, `configure`, stable
`observationKey`, and optional `dispose`. Each behavior owns its configuration
shape, payload validation, claim authority, and stable payload identity. The
core owns transport, revisioning, sequencing, retries, and bounds; it never
interprets a behavior payload and never accepts a module name from
configuration.

`src/behaviors/shell-outcomes.ts` is the only production behavior. It retains
`shell.created` facts, joins `shell.exited` by native shell ID, and emits a
terminal observation only after an exact ACP-confirmed claim arrives. Deletion
alone and missing exit evidence stay unknown.

The sender outbox applies one awaited, abortable exponential backoff to a failed
batch. A new enqueue cannot bypass that delay, and a fact leaves the queue only
after a durable host acknowledgement or an explicitly logged bounded gap.

Adding a behavior requires an explicit descriptor here and an explicit typed
receiver in `apps/backend/internal/agentctl/server/opencodeplugin`, plus
contract tests on both sides.

## Build and artifact

```bash
pnpm --filter @kandev/opencode-plugin typecheck
pnpm --filter @kandev/opencode-plugin test
pnpm --filter @kandev/opencode-plugin build
pnpm --filter @kandev/opencode-plugin check:artifact
```

`scripts/build.mjs` bundles the dependency-free ESM entry with esbuild and writes
the exact bytes and SHA-256 manifest to
`apps/backend/internal/agentctl/server/opencodeplugin/assets`. Go builds embed
that checked artifact. Do not edit the generated asset by hand.

## Packaged conformance

```bash
KANDEV_OPENCODE_CONFORMANCE=1 \
  pnpm --filter @kandev/opencode-plugin test:conformance
```

The harness installs the exact `@opencode/cli@2.2.0` package into a disposable
directory, or uses a binary named by
`KANDEV_OPENCODE_CONFORMANCE_CLI`. It verifies the pinned version, loads the
shipped artifact through inline `OPENCODE_CONFIG_CONTENT`, preserves a
file-configured user plugin, waits for native subscription readiness, claims one
background shell, and asserts the terminal exit code. It uses an isolated HOME,
XDG roots, workspace, database, and a local model stub.

Additional OpenCode versions require their own recorded conformance evidence
before joining the host's supported-version manifest.
