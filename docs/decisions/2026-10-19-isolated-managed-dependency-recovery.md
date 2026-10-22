# ADR-2026-10-19-isolated-managed-dependency-recovery: Repair missing bundled Codex dependencies privately

**Status:** accepted
**Date:** 2026-10-19
**Area:** backend, agentctl, protocol

## Context

A managed `codex-acp@2.2.2` installation contained `@openai/codex@0.160.1`
but lacked its macOS ARM64 platform package. The bridge returned an initialize
RPC error after its provider child exited. Running the cached Codex with
`--version` reproduced the failure independently of task resume. The user
repaired the installation and selected automatic recovery for future cases.
The reason npm omitted the dependency is not established.

[Bounded startup retry](2026-10-02-bounded-managed-npm-startup-retry.md)
correctly avoids deleting shared runtime files, but a metadata-refresh retry
does not restore an incomplete installed dependency tree. Reinstalling global
Codex does not repair the bridge's bundled copy. A bridge can remain alive, so
requiring the top-level process to exit would miss this failure.

## Decision

For a registered managed Codex ACP runtime, independently verify the host's
required bundled dependency is absent before permitting repair. Diagnostic
text identifies a candidate condition but supplies no executable, package,
version, path, or repair authority.

Prepare the same exact selected bridge in a fresh private project/cache on the
execution host, including optional platform packages. Validate it and retry
the original task initialization or capability operation once. A normal fresh
installation may resolve transitive versions within the bridge's declarations;
report the actual resulting bundled CLI observation instead of promising an
unchanged transitive version.

Keep the existing shared retry budget, deadlines, generation fences, process
cleanup, session identity, registry, and authorized profile context. Do not
delete or modify the shared broken tree, switch versions or `CODEX_PATH`, or
change a user's npm configuration. Confine optional-dependency inclusion and
private prefix/cache flags to this prepared runtime.

Prepared files belong to the executor-local agentctl process/instance and
outlive every subprocess that can use them. Backend detach/adoption is not a
cleanup event. Release only after owned descendants have stopped.

This qualifies the earlier decision's explicit-maintenance-only consequence
and strict ETARGET-only host capability-probe eligibility for this verified
condition, not its shared-tree safety rule. Other host probe failures remain
ETARGET-only. It also permits this narrow recovery during profile discovery
without granting general package-install or runtime-selection authority to
profile refresh.
The linked draft package must be implemented before these additions ship.

## Consequences

Users can resume and refresh capabilities without manually operating npm for
the confirmed missing-platform-dependency case. Healthy siblings remain
independent, including while another sibling exhausts its recovery.

Private preparation costs disk space and a cold npm installation, can still
fail under registry or policy constraints, and must honor cancellation and
sanitized diagnostics. No promise is made that npm failures cannot occur.
The initial scope is managed Codex ACP on supported task executors and host
discovery. No generic dependency installer, repaired-runtime promotion,
background preflight on every healthy launch, or crash-orphan janitor is added.

## Alternatives Considered

- Repeat the same npx command with online metadata: retains the incomplete
  execution tree and does not reliably restore missing native packages.
- Reinstall or remove the shared tree automatically: can break a healthy
  process still reading its files.
- Choose global Codex or a newer bridge: silently changes selected behavior
  and does not repair the selected bundled runtime.
- Offer only a repair button: safer than a terminal workaround, but the user
  explicitly chose automatic preparation and one retry.
- Install the package named in stderr directly: turns untrusted diagnostics
  into installation authority and does not establish runtime ownership.
- Persist and promote a repaired runtime generation: unnecessary selection,
  storage, and cross-executor coordination for this bounded correction.

## References

- [Requirements](../specs/agents/requirements/managed-npm-runtime-recovery.md)
- [Design](../specs/agents/system-design/managed-npm-runtime-recovery.md)
- [Fix package](../plans/managed-codex-dependency-recovery/plan.md)
