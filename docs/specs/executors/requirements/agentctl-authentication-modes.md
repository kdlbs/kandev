---
status: draft
system: executors
created: 2026-10-09
owners:
  - kandev
---

# Agentctl authentication modes

## Overview

Executor connections must retain their configured authentication mode across control and instance operations. A healthy controller must accept the same authorized client during agent startup.

Executors owns this contract because it defines how each execution environment connects to its controller. Platform recovery consumes the resulting runtime state.

## Terminology

- **Tokenless mode:** The controller starts without a bearer credential, as in the existing isolated Sprite launch.
- **Authenticated mode:** The controller starts with a bearer credential, including one generated through bootstrap nonce configuration.

## Requirements

### REQ-EXECUTORS-AGENTCTL-AUTH-MODES-001: Consistent authentication during executor startup

**Intent:** Supported executor connections can start agent work without weakening authenticated connections.

#### Acceptance criteria

- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.1:** In tokenless mode, the same client shall create an instance, configure its agent, and read its status without a bearer credential.
- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.2:** In tokenless mode, the client shall receive agent output and completion through the instance stream after it submits a prompt.
- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.3:** In authenticated mode, protected control and instance operations shall reject absent, malformed, and incorrect bearer credentials. Existing public health and bootstrap operations shall remain available.
- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.4:** In authenticated mode, valid credentials shall permit instance configuration, status access, and agent streaming. Credential rotation shall retain the existing single-driver restrictions.
- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.5:** A controller's authentication mode shall depend on its startup configuration. A missing request credential or failed authentication shall never select tokenless mode.
- **AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.6:** The correction shall preserve existing listener restrictions and Sprite transport isolation. It shall not require users to change executor profiles or provider credentials.

The authenticated rotation contract remains in
[standalone control-server single driver](standalone-control-server-single-driver.md).
This document does not extend standalone survival to remote executors.

## Out of scope

- A new authentication option or token distribution protocol for Sprites.
- New endpoint exemptions, network exposure, or ownership-adoption semantics.
- Changes to bootstrap error classification or recovery-card copy.
- Automatic recovery of previously failed sessions or replacement of retained Sprites.

## Related design

- [Authentication modes](../system-design/agentctl-authentication-modes.md)
