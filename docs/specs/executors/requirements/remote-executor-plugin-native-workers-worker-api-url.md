---
status: draft
system: executors
created: 2026-10-30
owners:
  - kandev
---

# Remote executor plugins: worker-side API URL requirements

## Overview

Continuation of [native worker capabilities](remote-executor-plugin-native-workers.md) (REQ-EXECUTORS-NATIVE-WORKER-004, moved here for the file size limit): the reachability probe of a `provider_forwarded` launch must request an address the worker can resolve. On a remote worker the host's own listener is not that address; the provider's forward is. The opt-in [capability](remote-executor-plugin-capability-versioning.md) `worker_api_url` lets the provider say so.

## Requirements

### REQ-EXECUTORS-NATIVE-WORKER-009: Worker-side API URL for the reachability probe

**Intent:** A launch on a worker that cannot reach the host's own address still proves the provider's forward, instead of failing with `api_unreachable` while the forward is healthy.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-009.1:** The capability `worker_api_url` (version 1) shall require `api_url_forward`. A provider that requires it can return `worker_api_url` in the provision resource descriptor, the loopback base URL of its forward on the worker.
- **AC-EXECUTORS-NATIVE-WORKER-009.2:** When `worker_api_url` is set, the host shall ask agentctl to probe `<worker_api_url>/api/v1/runtime/ping` instead of `<host_api_url>/api/v1/runtime/ping`. The probe token, attempts, budget and the `api_unreachable` rollback of AC-EXECUTORS-NATIVE-WORKER-004.9 are unchanged, and the bootstrap still carries `host_api_url`. When it is empty the probe uses `host_api_url` as before.
- **AC-EXECUTORS-NATIVE-WORKER-009.3:** The host shall accept only an absolute `http` URL with a loopback host (`127.0.0.1`, `::1` or `localhost`) and a port, and no credentials, path other than `/`, query or fragment. Any other value shall roll provisioning back before a probe runs.
- **AC-EXECUTORS-NATIVE-WORKER-009.4:** A provider that did not negotiate `worker_api_url`, or a launch that is not `provider_forwarded`, that returns `worker_api_url` shall be refused and rolled back before a probe runs.
