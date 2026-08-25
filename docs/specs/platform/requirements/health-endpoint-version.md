---
status: active
system: platform
created: 2026-08-07
owners:
  - nova28
---
# Health Endpoint — Surface the Running Version Requirements

## Overview

`GET /health` is the endpoint every operator already polls. Kubernetes liveness and
readiness probes (`k8s/deployment.yaml:39,47`), the CLI (`apps/cli/src/health.ts`),
the Go launcher (`internal/launcher/health.go:44`), the Homebrew service wrapper
(`scripts/release/kandev.rb`), the Tauri desktop shell
(`apps/desktop/src-tauri/src/backend.rs:555`), and the Playwright fixture
(`apps/web/e2e/fixtures/backend.ts:461`) all hit it. It answers "is this process up?"
but not "*which build* is up?", so an operator watching a rollout, a bad canary, or a
stuck upgrade cannot tell from their monitoring which version is answering.

The version is not reachable from an equivalent place today. `GET /api/v1/system/info`
does return it, but that route is **not** on the unauthenticated allowlist
(`internal/auth/httpmw/middleware.go:85-99`), while `/health` **is**. Once auth is
enabled, a monitoring system must be issued a credential purely to read a version
string. Adding the field to `/health` closes that gap for every unauthenticated prober
without weakening any existing boundary.

**Correction to the originating request.** The request stated that `kandev_version` is
"already on `/system/info`". That is not accurate, and the difference matters because it
determines the field name this spec freezes. `GET /api/v1/system/info` returns a field
named **`version`** (`internal/system/info/info.go:19`), not `kandev_version`. The
identifier `kandev_version` does exist in this codebase, but for two unrelated things:
the `kandev_meta.kandev_version` database key that drives upgrade-backup safety
(`internal/persistence/meta.go:83`, ADR `0008-db-upgrade-safety.md`), and the
share-snapshot payload field (`internal/task/share/snapshot.go:19`). Both names
therefore had prior art. The name was resolved by explicit decision — see
[Decisions](#decisions).

## Requirements

### REQ-PLATFORM-HEALTH-ENDPOINT-VERSION-001: Health Endpoint — Surface the Running Version

**Intent:** `GET /health` is the endpoint every operator already polls. Kubernetes liveness and
readiness probes (`k8s/deployment.yaml:39,47`), the CLI (`apps/cli/src/health.ts`), the Go launcher
(`internal/launcher/health.go:44`), the Homebrew service wrapper (`scripts/release/kandev.rb`), the
Tauri desktop shell (`apps/desktop/src-tauri/src/backend.rs:555`), and the Playwright fixture
(`apps/web/e2e/fixtures/backend.ts:461`) all hit it. It answers "is this process up?" but not
"*which build* is up?", so an operator watching a rollout, a bad canary, or a stuck upgrade cannot
tell from their monitoring which version is answering. The version is not reachable from an
equivalent place today. `GET /api/v1/system/info` does return it, but that route is **not** on the
unauthenticated allowlist (`internal/auth/httpmw/middleware.go:85-99`), while `/health` **is**. Once
auth is enabled, a monitoring system must be issued a credential purely to read a version string.
Adding the field to `/health` closes that gap for every unauthenticated prober without weakening any
existing boundary. **Correction to the originating request.** The request stated that
`kandev_version` is "already on `/system/info`". That is not accurate, and the difference matters
because it determines the field name this spec freezes. `GET /api/v1/system/info` returns a field
named **`version`** (`internal/system/info/info.go:19`), not `kandev_version`. The identifier
`kandev_version` does exist in this codebase, but for two unrelated things: the
`kandev_meta.kandev_version` database key that drives upgrade-backup safety
(`internal/persistence/meta.go:83`, ADR `0008-db-upgrade-safety.md`), and the share-snapshot payload
field (`internal/task/share/snapshot.go:19`). Both names therefore had prior art. The name was
resolved by explicit decision — see [Decisions](#decisions).

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
