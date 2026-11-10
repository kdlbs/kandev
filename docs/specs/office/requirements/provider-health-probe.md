---
status: active
system: office
created: 2026-11-09
owners:
  - kandev
---

# Office Provider Health Probe Requirements

## Overview

An operator can ask Office to retry a degraded provider. Office answers by probing
the provider's ACP command. The probe must run the command the way a real agent
launch runs it. A probe that fails where a launch succeeds marks a working provider
as needing user action and parks every queued Office run for it.

## Terminology

- **Probe:** the single ACP `initialize` round trip Office makes to decide whether a
  provider is healthy.
- **Managed npm prefix marker:** the trusted placeholder
  `~/.kandev/managed-npm-runtime` that an npm-based agent command carries after
  `--prefix`. Every launch replaces it with a private temporary root.

## Requirements

### REQ-OFFICE-PROVIDER-PROBE-001: The probe launches a provider like a real launch

**Intent:** A health decision is only trustworthy when it reflects what a launch
would do.

**User story:** As an operator, I want a provider retry to succeed whenever a real
run would launch, so that a working provider is not parked.

#### Acceptance criteria

- **AC-OFFICE-PROVIDER-PROBE-001.1:** When the probe command carries the managed npm
  prefix marker, the system shall replace the marker with the private temporary root
  before it starts the process, and the process shall never see the marker.
- **AC-OFFICE-PROVIDER-PROBE-001.2:** When the probe prepares the private temporary
  root, the system shall create it, if absent, as a directory with mode 0700.
- **AC-OFFICE-PROVIDER-PROBE-001.3:** When the probe command does not carry the
  marker, the system shall start it with its arguments unchanged.
- **AC-OFFICE-PROVIDER-PROBE-001.4:** When the system cannot prepare the private
  temporary root, the probe shall report a classified provider failure and shall not
  start the process.
- **AC-OFFICE-PROVIDER-PROBE-001.5:** The probe shall not change the command arguments
  held by the component that resolved them.

## Exclusions

- How a transient provider error escalates to `user_action_required`.
- Whether cancelled or archived tasks keep retrying.
