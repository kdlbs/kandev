---
status: active
system: executors
created: 2026-10-22
owners:
  - tbd
---

# OpenCode V2 Credential Provisioning

## Overview

Operators can provision OpenCode accounts into isolated executor sessions without
copying provider databases or installing a credential-bootstrap wrapper.

## Requirements

### REQ-EXECUTORS-OPENCODE-AUTH-001: Session credential provisioning

#### Acceptance criteria

- **AC-EXECUTORS-OPENCODE-AUTH-001.1:** When the OpenCode credential method is selected for a managed V2 session lacking saved accounts, the system shall bootstrap host accounts before starting ACP, including subsequent sessions and stopped-session starts with an empty credential store.
- **AC-EXECUTORS-OPENCODE-AUTH-001.2:** Repeated provisioning shall preserve existing destination credential values and active-account selections, including refreshed OAuth credentials.
- **AC-EXECUTORS-OPENCODE-AUTH-001.3:** When selected provisioning fails or the selected V2 version is older than 2.1.2, launch shall fail without starting ACP or exposing credential values in errors.
- **AC-EXECUTORS-OPENCODE-AUTH-001.4:** The system shall retain V1 credential-file behavior and the existing remote credential selection. Unselected remote methods and standalone sessions shall not export host accounts.
- **AC-EXECUTORS-OPENCODE-AUTH-001.5:** Cancellation shall terminate provisioning subprocesses and release the session for a later attempt. Concurrent configuration, agent startup, and credential imports shall not change a session during import.
- **AC-EXECUTORS-OPENCODE-AUTH-001.6:** Credential exports shall be bounded to 1 MiB and shall not be transported through subprocess arguments, environment variables, or temporary export files.
- **AC-EXECUTORS-OPENCODE-AUTH-001.7:** A session with established destination accounts and no pending bootstrap state shall restart without exporting controller credentials or requiring controller authentication. An unreadable destination store shall fail closed rather than be treated as empty. Saved-account presence shall not be reported as proof of provider acceptance.
- **AC-EXECUTORS-OPENCODE-AUTH-001.8:** Controller account changes and repeated bootstrap shall not replace destination credentials or switch destination selections. Operators shall receive a documented session-local reauthentication and account-switching path that preserves conversation history and unrelated accounts.
- **AC-EXECUTORS-OPENCODE-AUTH-001.9:** When selected V2 provisioning encounters an unsupported runtime source, launcher, or worker capability, admission shall fail with an actionable compatibility error before exporting controller credentials. The minimum version shall apply to this provisioning operation, not unrelated launches.
- **AC-EXECUTORS-OPENCODE-AUTH-001.10:** Bootstrap shall use the explicitly resolved controller credential environment and the final destination session environment. Destination HOME/XDG overrides shall never redirect the controller credential source.
- **AC-EXECUTORS-OPENCODE-AUTH-001.11:** After an interrupted import or lost acknowledgement, a retry shall preserve already imported or refreshed credentials and destination selections; an incomplete bootstrap shall not be mistaken for a completed one.

## Validation boundary

Independent OAuth copies require provider-specific live evidence. Synthetic merge
tests alone shall not be presented as proof that controller and executor copies
remain usable through actual expiry and concurrent refresh. Release evidence
must identify tested CLI versions, provider, executors, refresh outcomes, and
limitations. A failed live refresh test blocks claiming support for that usage.

## Exclusions

Volatile Kubernetes bootstrap recovery, provider-history durability, runtime cache
lifecycle, and deployment-specific workaround retirement are separate work.

## System design

See [OpenCode V2 credentials](../system-design/opencode-v2-credentials.md).
