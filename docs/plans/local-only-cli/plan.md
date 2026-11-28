---
created: 2026-11-26
status: done
requirements:
  - REQ-LAUNCHER-LOCAL-ONLY-001
system_design:
  - ../../specs/launcher/system-design/local-only-cli.md
---

# Implementation plan for local-only CLI startup

## Overview

Refactor open PR #5518 from documentation guidance into a runtime default.
Keep its verified first-task tutorial. Use a single work order because the
listener, launch URL, compatibility defaults, and public guidance must agree
when the change ships.

## Scope

- Change the standalone bind fallback and preserve browser origin.
- Preserve explicit bind semantics and the official container and service
  fallbacks. Cover upgrades with disposable data.
- Replace inline loopback settings in first-run examples that use the new
  default. Keep explicit settings where a deployment requires them.

## Technical approach

Start with focused failing tests in `internal/common/config` and
`internal/launcher`. Add a subprocess socket check for the default listener,
then verify the actual native bundle and official image. Keep the production
change in the shared bind resolver and launcher endpoint mapping. Keep the
official image marker below operator configuration precedence.

Reconcile the PR branch with live `main` before implementation. Keep the
existing PR and update it after scoped tests pass. Preserve the user's
restrictions on merging, queueing, and auto-merge.

## Work order

- [x] [Implement and verify local-only startup](task-01-local-only-startup.md)

## Verification results

The configuration and launcher suites, public docs checks, and specifications
passed. A disposable v0.103.1 upgrade retained data and the localhost origin.
The native listener and official container passed the deployment checks.
See the work order for test details.

## Risks

An explicit IPv4 loopback bind and the implicit default have different browser
URL compatibility requirements. An image-level public host override would mask
YAML bind settings. Old standalone installs that rely on a wildcard need clear
upgrade instructions. Test the marker from an already-generated service unit.
