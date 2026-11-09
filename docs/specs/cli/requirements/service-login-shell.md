---
status: draft
system: cli
created: 2026-10-26
owners:
  - kandev
---

# Service login shell

## Overview

An operator can opt a macOS user service into non-interactive login initialization.
This supplies exported tool paths and environment values before Kandev starts.
CLI owns the selection, installation, and update continuity contract.
Platform retains shared environment normalization and local executable discovery.

## Requirements

### REQ-CLI-SERVICE-LOGIN-SHELL-001: Explicit service shell initialization

**Intent:** Tools configured by login startup files remain available to a service
without changing startup behavior for existing installations.

#### Acceptance criteria

- **AC-CLI-SERVICE-LOGIN-SHELL-001.1:** Without an explicit selection or a previously
  saved selection, service startup shall retain its current direct-launch behavior.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.2:** A macOS user-service installation shall accept
  `--shell <path-or-name>` and `--shell=<path-or-name>` for Bash or Zsh.
  Selection shall resolve to an absolute executable during installation.
  Empty values, unsupported shells, unavailable executables, other platforms,
  and system-service use shall fail before service mutation.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.3:** An opted-in service shall run the selected
  shell's non-interactive login initialization before normal Kandev startup.
  Discovery, capability probes, and new local agents shall inherit exported
  tool paths and environment values through their existing environment rules.
  Shell aliases and functions shall not become executable tools.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.4:** Shell initialization shall use the runtime
  user's account home and selected shell, even when inherited `HOME` or `SHELL`
  is absent or incorrect. Explicit service configuration shall retain precedence.
  Kandev's data directory shall not select the account's startup files.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.5:** Reinstallation without a shell argument,
  service restart, and supported Apply updates shall preserve the saved selection.
  `--shell=none` shall remove the selection and restore direct startup.
  Legacy installations without a selection shall remain compatible.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.6:** The native Kandev executable shall remain
  the configured service entrypoint. Successful shell initialization shall leave
  no extra persistent shell supervisor. Existing shutdown and restart behavior
  shall remain available.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.7:** Invalid saved selections or inconsistent
  service metadata shall fail with actionable errors rather than silently disable
  shell initialization. A missing shell at startup shall fail visibly.
  If a startup file exits before Kandev starts, the service shall not report readiness.
- **AC-CLI-SERVICE-LOGIN-SHELL-001.8:** Documentation shall describe selection,
  reset, update continuity, and recovery from startup-file failures.
  It shall distinguish login initialization from an interactive terminal.
  Kandev shall not enable interactive mode or edit startup files automatically.

## Out of scope

Linux services, macOS system services, Windows, other shells, automatic shell
selection, interactive startup, terminal aliases, and project-specific mise
activation are excluded from this delivery. Remote executors retain their own
environment contracts. No rendered UI changes or runtime feature flag are needed.

## Implementation plans

- [Service login shell fix](../../../plans/service-login-shell/plan.md)
