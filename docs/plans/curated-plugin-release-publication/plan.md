---
owners:
  - kandev
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
  - REQ-PLUGINS-MARKETPLACE-002
system_design:
  - ../../specs/plugins/system-design/marketplace.md
created: 2026-08-30
status: building
---

# Implementation Plan: Curated plugin release publication

## Overview

Close the release-visibility gap in the official plugin marketplace: a valid
release published by an already-curated plugin repository becomes discoverable
through the central index within the documented 10-minute target under normal
GitHub Actions scheduling, without waiting for the 06:00 UTC rebuild, while
curation authority stays with the maintainer-reviewed allowlist and package
integrity stays with the central verifier.

The design keeps polling and deployment credentials inside `kdlbs/kandev`,
enumerates repositories only from the checked-out `plugin-registry/plugins.yaml`,
and serializes every official Pages deployment behind one coalescing
concurrency group.

## Requirement coverage

| Requirement | Covered by |
| --- | --- |
| `REQ-PLUGINS-MARKETPLACE-001` | Existing marketplace catalog behavior, retained by [task-01-prompt-publication](task-01-prompt-publication.md). |
| `REQ-PLUGINS-MARKETPLACE-002` | Central release poll, builder validation, fallback retention, and shared concurrency, delivered by [task-01-prompt-publication](task-01-prompt-publication.md). |

## Work packages

- [x] [task-01-prompt-publication](task-01-prompt-publication.md) - central
  five-minute curated release poll, index builder validation and fallback
  retention, package verifier, shared deployment serialization, and update
  ordering semantics.

## Verification

Focused suites:

```bash
node --test plugin-registry/check-releases.test.mjs plugin-registry/build-index.test.mjs plugin-registry/workflow-contract.test.mjs
cd apps/backend && go test ./cmd/plugin-package-verify/ ./internal/plugins/pkgtar/ ./internal/plugins/manifest/
```

The full repository gates (`make -C apps/backend test`, web unit suites,
typecheck, lint, i18n) must pass or be classified against a clean upstream
baseline before delivery.
