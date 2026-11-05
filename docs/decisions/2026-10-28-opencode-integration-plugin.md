# ADR-2026-10-28-opencode-integration-plugin: One owned OpenCode v2 integration plugin

**Status:** accepted
**Date:** 2026-10-28
**Area:** protocol, backend

## Context

OpenCode 2.2.0 records background-shell results internally but omits them from
its ACP projection. Kandev retains launch identities and later reports unknown
activity. Upstream fixes remain incomplete for shell outcomes and automatic
parent responses.

The user requested a Kandev-supplied OpenCode v2 plugin as a compatibility path.
They also requested automatic launch integration and a foundation for future
Kandev behaviors. OpenCode v2 has a server-plugin API with scoped setup, cleanup,
configuration, and native event subscriptions.

The existing full-continuity package excludes side channels. This decision
permits one bounded plugin channel for declared behaviors. ACP remains the
conversation transport. The full-continuity producer gate remains in force.

## Decision

Supply one private `@kandev/opencode-plugin` artifact with the `kandev` plugin ID.
Keep its source in this repository because it implements the host's provider
integration contract. It is not a marketplace plugin for the Kandev runtime.

Compile the plugin to ESM and embed its reviewed artifact in agentctl. Agentctl
materializes it in the owning executor and appends it through launch-only
OpenCode configuration. It never edits the user's plugin installation or global
configuration. It performs no package download during plugin activation.

Use OpenCode v2's `Plugin.define({ id, setup })` server API. Require actual
runtime compatibility evidence and a successful readiness handshake. Do not
inject a v1 hook export or select a runtime from the agent display name.

Keep a small static registry of behavior modules. Share configuration,
authentication, limits, delivery, and cleanup. Each module owns its typed payload
decoder and version. Future modules require code, requirements, and tests. The
registry does not load arbitrary modules or provide a general RPC command bus.

Agentctl owns a dedicated loopback callback listener and a credential for each
launch lease. The credential grants only the declared plugin protocol. It grants
no access to agentctl's control, shell, filesystem, MCP, or process routes.
Bootstrap data stays in a private runtime file outside the workspace.

The first module reports terminal shell outcomes for ACP-confirmed background
launches. Claims come from Kandev's original launch bindings. A bounded native
cache handles outcomes that precede claims. Accepted observations enter the
existing ordered agentctl journal before acknowledgement.

This first module does not establish continuing liveness. Existing unverified
jobs can still become unknown after a foreground prompt. Actual terminal
evidence can settle them later. It does not restore automatic assistant text,
stream output, add controls, or change admission or workflow behavior.

## Consequences

Kandev can settle new shell jobs without an upstream release or a private
database reader. New behaviors reuse one installation and ownership lifecycle.
The same artifact travels with local and remote agentctl binaries.

The integration adds a callback protocol, bounded retry state, and packaging
checks. Compatibility evidence must cover the packaged CLI and artifact.
Plugin failure preserves ordinary ACP behavior but can leave a job unknown.
Existing historical unknown jobs require a separately authorized recovery path.

One source owns each behavior. A proven equivalent ACP capability takes
precedence and disables the matching compatibility module. Other modules can
remain active. Keep the release toggle until later promotion and retirement.

## Alternatives considered

- Wait for upstream: preserves one channel but leaves completed jobs unknown.
- Patch or fork OpenCode: couples Kandev releases to a separate CLI build and
  runtime-selection policy.
- Poll the default OpenCode server: ACP owns a separate private server and
  credential. The default server does not establish ownership of its jobs.
- Read SQLite or output files: depends on private storage and cannot prove
  liveness from missing rows or files.
- One plugin per behavior: duplicates installation, ownership, and delivery.
- General plugin command bus: grants unnecessary authority and adds speculative
  extension contracts.
- Treat unknown jobs as complete: invents outcomes. Changing the badge alone
  does not deliver the missing results.

## Related contracts

- [Plugin requirements](../specs/agents/requirements/opencode-kandev-plugin.md).
- [Plugin design](../specs/agents/system-design/opencode-kandev-plugin.md).
- [Shared background-work decision](2026-09-27-provider-neutral-background-work.md).
- [OpenCode runtime selection](2026-09-27-opencode-runtime-adoption.md).
- [Full continuity package](../plans/opencode-background-continuity/plan.md).
