---
status: active
system: ui
created: 2026-10-05
owners:
  - kandev
---

# Automatic preview URL detection requirements

## Overview

Preview surfaces select a local development-server URL from process output.
Invalid port announcements must not replace a working candidate or become a
different port by truncation. UI owns this reusable presentation selection
contract across preview surfaces; executor admission and task feedback storage
retain their separate owners.

## Terminology

- **Full candidate:** An HTTP or HTTPS URL using the existing supported local
  host forms: `localhost`, `127.0.0.1`, or `0.0.0.0`.
- **Bare candidate:** A supported local host followed by a colon and a complete
  two-to-five-digit decimal port token, without requiring a scheme.
- **Numeric port boundary:** 0 through 65535 inclusive. This preserves existing
  detector acceptance of zero and leading zeros. Detection is not permission
  to proxy a port or evidence that a service is listening.

## Requirements

### REQ-UI-PREVIEW-URL-DETECTION-001: Complete preview port selection

**Intent:** A valid announced preview remains available when output also contains
an invalid numeric port.

#### Acceptance criteria

- **AC-UI-PREVIEW-URL-DETECTION-001.1:** When a full or bare candidate announces
  a numeric port above 65535, the system shall ignore that candidate. Bare
  candidates shall use the entire contiguous digit token and shall never select
  a prefix of an overlong token such as `123456` as port `12345`.
- **AC-UI-PREVIEW-URL-DETECTION-001.2:** Valid candidates at port 65535 shall
  remain selectable in full and bare forms. Existing lower-bound acceptance,
  bare digit-width limits, supported hosts, HTTP/HTTPS behavior, path/query/hash
  handling, and ANSI-bearing output behavior shall be preserved. Missing-port
  and default-port normalization behavior shall retain existing semantics.
- **AC-UI-PREVIEW-URL-DETECTION-001.3:** Within a line the system shall select
  the first successfully parsed full candidate. If none succeeds, it shall
  select the last valid bare candidate. Invalid numeric candidates before or
  after valid candidates shall not alter that preference.
- **AC-UI-PREVIEW-URL-DETECTION-001.4:** Across output lines the system shall
  retain the last valid selection. A later line containing only invalid numeric
  candidates shall not erase an earlier selection. When the earlier URL was
  eligible for the existing session proxy rewrite, the same rewritten preview
  shall remain available, including its path, query, and fragment.
- **AC-UI-PREVIEW-URL-DETECTION-001.5:** Desktop and phone preview consumers
  shall share the same candidate eligibility and selection behavior. This
  correction shall introduce no control, copy, layout, scrolling, navigation,
  or viewport-dependent interaction change.

## Out of scope

- Backend proxy admission, including its separate 1024..65535 range,
  authorization, transport, or port discovery policy.
- Changes to manual URL entry, manual port opening, task feedback capture,
  supported host grammar, malformed nonnumeric URL syntax, or URL security rules.
- Treating zero, privileged ports, or default ports as newly proxyable.
- New APIs, schemas, dependencies, persistence, runtime flags, or UI surfaces.

## Related contracts

- [Manual proxy opening](port-proxy-browser-panel.md).
- [Task-owned preview feedback](../../tasks/requirements/web-preview-feedback.md).
- [Paired design](../system-design/preview-url-detection.md).
