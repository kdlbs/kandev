---
status: active
system: platform
created: 2026-08-07
owners:
  - nova28
---
# Health Endpoint — Surface the Running Version Requirements

## Overview

`GET /health` is the endpoint every operator already polls. Kubernetes liveness and readiness probes (`k8s/deployment.yaml:39,47`), the CLI (`apps/cli/src/health.ts`), the Go launcher (`internal/launcher/health.go:44`), the Homebrew service wrapper (`scripts/release/kandev.rb`), the Tauri desktop shell (`apps/desktop/src-tauri/src/backend.rs:555`), and the Playwright fixture (`apps/web/e2e/fixtures/backend.ts:461`) all hit it. It answers "is this process up?" but not "*which build* is up?",...

## Requirements

### REQ-PLATFORM-HEALTH-ENDPOINT-VERSION-001: Health Endpoint — Surface the Running Version

**Intent:** `GET /health` is the endpoint every operator already polls. Kubernetes liveness and readiness probes (`k8s/deployment.yaml:39,47`), the CLI (`apps/cli/src/health.ts`), the Go launcher (`internal/launcher/health.go:44`), the Homebrew service wrapper (`scripts/release/kandev.rb`), the Tauri desktop shell (`apps/desktop/src-tauri/src/backend.rs:555`), and the Playwright fixture (`apps/web/e2e/fixtures/backend.ts:461`) all hit it. It answers "is this process up?" but not "*which build* is up?",...

#### Acceptance criteria

- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.1:** `GET /health` SHALL include the running Kandev version in its JSON response body.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.2:** The field SHALL be named **`version`**, matching `GET /api/v1/system/info`.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.3:** The field SHALL be present in **both** the ready (200) and not-ready (503) responses, so an operator can identify the build of a backend that is stuck starting.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.4:** The field value SHALL be the same string `GET /api/v1/system/info` reports as `version` for the same running process.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.5:** The field SHALL be served to unauthenticated callers, exactly as the rest of the `/health` payload already is. This is a deliberate, accepted disclosure — see [Security](#security-and-permissions).
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.6:** Every existing field (`status`, `service`, `mode`) SHALL retain its current name, value, and semantics. This is a purely additive change.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.7:** The existing HTTP status semantics (200 when ready, 503 while starting) SHALL be unchanged.
- **AC-PLATFORM-HEALTH-ENDPOINT-VERSION-001.8:** The desktop health-token response header SHALL continue to be set on the 200 path only, unchanged.

## System design

The migrated technical source is split into [part 1](../system-design/health-endpoint-version.md).
