---
id: "12-public-canvas-documentation"
title: "Public canvas documentation"
status: done
wave: 12
depends_on:
  - "11-workspace-mobile-surfaces"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-004
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-002
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.1
  - AC-CANVASES-AGENT-WEB-APPS-001.6
  - AC-CANVASES-AGENT-WEB-APPS-002.1
  - AC-CANVASES-AGENT-WEB-APPS-002.6
  - AC-CANVASES-AGENT-WEB-APPS-003.1
  - AC-CANVASES-AGENT-WEB-APPS-004.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.7
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.3
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 12: Public canvas documentation

## Summary

Publish the user, author, operator, and security contracts after the behavior is
implemented. Keep reference, how-to, and explanation content in their owning
public pages.

## In scope

- Document `ui.web_apps` in the plugin manifest reference.
- Document sandbox, capability, network, and opaque-storage behavior.
- Add a canvas how-to for agent creation, promotion, permission review, and
  Quick Chat editing.
- Document `features.canvases` and its restart behavior.
- Document database-only backup limits and the artifact-directory boundary.
- Update feature status and public navigation.
- Validate public docs, links, examples, and metadata.

## Out of scope

- Marketplace publication and general custom-plugin UI slots.

## Acceptance

- Authors can build a package without relying on an injected JavaScript API.
- Operators can identify the complete backup and restore boundary.
- Users can create, promote, edit, recover, and remove a canvas from the docs.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/check-links.py docs/public
```

## Files likely touched

- `docs/public/plugins-manifest.md`
- `docs/public/plugins-authoring.md`
- `docs/public/security.md`
- `docs/public/configuration.md`
- `docs/public/operations.md`
- `docs/public/feature-status.md`
- `docs/public/canvases.md`
- `docs/public/meta.json`

## Dependencies

- Task 11 completes the final user-visible behavior and terminology.

## Risks

- Database-only backup wording can imply that artifacts are recoverable.
- Authoring examples can depend on browser storage that opaque origins deny.

## Parallelism

`sequential`

## Inputs

- Implemented manifest, security, authoring, recovery, and user-flow contracts.
- Current plugin, security, operations, and configuration public pages.

## Results

Published the canvas authoring guide and updated plugin manifest, security,
configuration, operations, feature-status, and public navigation content.

Verification:

- `node --test scripts/validate-public-docs.test.mjs` — 61 tests passed.
- `node scripts/validate-public-docs.mjs` — 42 published pages validated,
  including local and heading links.
- `python3 scripts/lint-spec-files.py --all` — passed.
- The referenced `scripts/check-links.py` helper is not present in current
  `main`; the public-doc validator provides the available link check.
