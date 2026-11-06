---
status: current
system: cli
requirements:
  - REQ-CLI-NATIVE-KANDEV-CLI-001
---

# Native Service Configuration System Design

## Purpose and boundaries

CLI owns this contract because `kandev service install` translates selected
startup configuration into native service definitions. This design covers only
environment serialization in that translation. It does not redesign the native
CLI or claim coverage of all criteria in the broader draft requirement.

Typed configuration selection and provenance remain with
[Platform startup configuration](../../platform/requirements/startup-configuration-parity.md).
Service identity and ownership retain
[the accepted continuity policy](../../../decisions/2026-07-31-system-service-user-continuity.md).
No new architecture decision is needed to follow the service manager's existing
serialization grammar.

## Requirement mapping

| Requirement criteria | Design sections |
| --- | --- |
| `REQ-CLI-NATIVE-KANDEV-CLI-001`, AC 001.9 and 001.10 | Environment serialization; Compatibility and persistence; Verification |
| `REQ-CLI-NATIVE-KANDEV-CLI-001`, AC 001.11 | PATH trust boundary; Verification |

## Components and responsibilities

`apps/backend/internal/launcher/service.go` owns `renderSystemdUnit`, shared by
Linux user and system services, and `renderLaunchdPlist` for macOS. Its
`nativeServiceUnitInput` carries already-selected configuration and detected PATH
prefixes. Selection, omission, ordering, metadata path derivation, and filesystem
validation stay with their current owners.

## Environment serialization

Each emitted `Environment=` assignment quotes the whole `key=value` when
needed, preserving its assignment boundary and escaping double quotes and
backslashes according to systemd unit syntax. Literal dollars pass through
unchanged, including adjacent dollars and variable-shaped text such as `$USER`
and `${HOME}`. Ordinary values escape each literal percent as `%%` so specifier
expansion yields the original percent.

`serviceEnvLine` and `serviceEnvLineAllowSpecifiers` must select environment
quoting explicitly instead of inheriting command-dollar substitution from
`quoteForUnit`. Keep this distinction local to the renderer; a small shared
escaping implementation is acceptable if its callers express both contexts
clearly. Do not add a parser, external dependency, or test-only production API.

`ExecStart=` continues to use `quoteForUnit` with its existing dollar-to-`$$`,
percent-to-`%%`, quote, and backslash escaping. The public executable and
`--headless` argv remain unchanged. The correction cannot be a global removal
of dollar escaping from command and environment contexts.

The authoritative
[systemd Environment reference](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#Environment=)
defines environment assignments without variable expansion, while still
applying quoting and specifier expansion. Command serialization is a distinct
context described by
[systemd command lines](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html#Command%20lines).

## PATH trust boundary

`servicePathWithPrefixes` combines accepted prefixes with `systemdServicePath`.
`cleanServicePathDirs` already rejects percent-bearing prefixes, alongside its
other existing exclusions. Preserve that filter. Only the existing trusted base
PATH contributes intentional `%h` specifiers; the PATH environment quoting path
keeps those specifiers while preserving dollars in accepted prefixes. It does
not evaluate `$PATH` or other shell substitutions.

## Compatibility and persistence

The renderer change applies to both Linux scopes without changing `User=`,
targets, commands, lifecycle settings, installation identity, ownership checks,
or update policy. Existing generated units are not rewritten automatically by
this correction; their normal installation/update lifecycle remains unchanged.
There is no schema, API, SDK, configuration default, flag, or data migration.

`renderLaunchdPlist` continues using structured `ProgramArguments` and
`escapeXML` for environment strings. Dollars and percents there remain literal;
XML metacharacters retain existing escaping. No macOS production change is
needed. No service-manager, shell-injection, script-template, startup recovery,
or database behavior is added.

## Verification

Portable tests exercise the full native renderers and compare exact rendered
assignment lines against independently authored expectations. Include both
Linux scopes, ordinary and variable-shaped dollars, adjacent dollars, spaces,
quotes, backslashes, literal percents in ordinary values, and trusted PATH
specifiers alongside a dollar-bearing accepted prefix. Test home, selected
configuration, derived metadata, and bundle paths. Preserve configuration-owned
home omission and existing PATH prefix rejection. Compare `ExecStart=` and
launchd output as compatibility controls.

These tests guard the serialization boundary without implementing a substitute
unit parser or requiring a live systemd user bus. External manager evidence and
its limits belong to the repair plan. They do not establish actual installed
Kandev startup or database effects.
