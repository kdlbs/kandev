---
status: active
system: platform
created: 2026-11-04
owners:
  - kandev
---

# Source frontend build mode requirements

## Overview

Contributors need a production React runtime during long debug sessions with Kandev diagnostics enabled.
An inherited Node environment must not change a production source build into a development React build.

Platform owns this contract because it owns source startup and diagnostics.
Release retains package publication, and UI retains presentation and interaction contracts.

## Requirements

### REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001: Deterministic production React builds

**Intent:** Source startup selects the intended React runtime independently of Kandev diagnostic settings.

#### Acceptance criteria

- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.1:** When `NODE_ENV=development` is inherited, `make start`, `make start-verbose`, and `make start-debug` shall build production React.
- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.2:** A debug source start shall retain Kandev frontend debug logging, agent message dumps, and pprof with production React.
  Backend file logging shall retain [AC-PLATFORM-DIAGNOSTIC-LOGGING-001.4](diagnostic-logging.md#acceptance-criteria).
- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.3:** The production build-mode selection shall apply only to frontend compilation.
  It shall not introduce production-only dependency installation or remove development dependencies needed for compilation.
- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.4:** The shared production web builds and native Windows source-start counterparts shall select the same production React runtime.
  The E2E web build shall retain its explicit pseudo-locale inclusion.
- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.5:** With an unset or development-valued `NODE_ENV`, `make dev` and `make dev-web` shall continue to use development React.
  A production source build shall not change the invoking environment or subsequent development launches.
- **AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.6:** Without an explicit profiling opt-in, production source builds shall omit React development warnings and default React performance-track instrumentation.
  Kandev frontend debug logging shall remain available independently of that instrumentation.

## Compatibility

The existing [debug-launch profile decision](../../../decisions/2026-08-10-debug-launcher-profile-selection.md) remains authoritative for profile selection and title defaults.
Explicit backend log thresholds retain their existing precedence.
This contract does not add a profiling command or change runtime feature flags.

## Out of scope

- A guarantee that all browser memory growth or renderer crashes disappear.
- Automatic removal of performance entries or changes to console retention.
- Normalizing an inherited production `NODE_ENV` for the Vite development server.
- Changes to direct package-manager build commands, release publication, or dependencies.

## System design

See [Source frontend build mode](../system-design/source-frontend-build-mode.md).
