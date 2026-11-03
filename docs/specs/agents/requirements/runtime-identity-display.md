---
status: active
system: agents
created: 2026-10-22
owners:
  - Kandev
---

# Agent runtime identity display requirements

## Overview

Settings agent cards should explain which runtime new structured sessions use.
A detected local executable is troubleshooting evidence and can differ from that runtime.
Agents owns runtime identity; Platform owns shared settings discovery.
This capability complements runtime update management without changing its ownership rules.

## Terminology

- **Session runtime:** The runtime selected for subsequent structured agent sessions.
- **Detected CLI:** A local executable discovered on the Kandev host.
- **Diagnostics:** An expandable, initially closed region containing discovery evidence.

## Requirements

### REQ-AGENTS-RUNTIME-IDENTITY-001: Understand session runtime identity

**Intent:** Users can identify the runtime Kandev selects without confusing it with a local installation.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-IDENTITY-001.1:** When Kandev selects a managed npm runtime, the card shall identify the session runtime as managed by Kandev and show its package and effective selected version. A detected native CLI shall not replace this identity.
- **AC-AGENTS-RUNTIME-IDENTITY-001.2:** When a native or harness-owned runtime is selected, the card shall identify that source without claiming that a managed fallback is the selected host runtime. Custom or unsupported runtimes shall not be described as managed npm runtimes.
- **AC-AGENTS-RUNTIME-IDENTITY-001.3:** A detected CLI path shall appear only inside initially closed diagnostics. Expanding diagnostics shall show the path and any existing CLI version observation explicitly as local discovery evidence. The disclosure shall be omitted when no such evidence exists.
- **AC-AGENTS-RUNTIME-IDENTITY-001.4:** Missing or unrecognized runtime identity shall show an explicit unknown session runtime. Missing versions shall remain unknown; local CLI observations shall not supply a managed runtime version. A selected runtime shall not imply a successful launch or authentication.
- **AC-AGENTS-RUNTIME-IDENTITY-001.5:** Desktop and phone users shall see the same identity and access diagnostics without hover. Phone disclosure controls shall provide at least 44px touch targets; long package names and paths shall remain readable without horizontal document overflow. Copy shall be localized in all shipped locales.
- **AC-AGENTS-RUNTIME-IDENTITY-001.6:** Viewing identity and diagnostics shall not initiate installation, updates, authentication, or runtime selection changes. Labels describe subsequent structured sessions, not existing processes, passthrough commands, or executor-owned remote installations.

## Out of scope

Changing executable selection, runtime update policy, availability detection, authentication,
model discovery, running sessions, or executor installation ownership.
