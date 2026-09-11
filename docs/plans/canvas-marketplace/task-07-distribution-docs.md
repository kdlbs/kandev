---
id: "07-distribution-docs"
title: "Document canvas distribution"
status: done
wave: 7
depends_on:
  - "06-sharing-ui"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-005
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-002.1
  - AC-PLUGINS-MARKETPLACE-002.2
  - AC-PLUGINS-MARKETPLACE-002.3
  - AC-PLUGINS-MARKETPLACE-002.4
  - AC-CANVASES-MARKETPLACE-001.1
  - AC-CANVASES-MARKETPLACE-001.2
  - AC-CANVASES-MARKETPLACE-001.3
  - AC-CANVASES-MARKETPLACE-002.1
  - AC-CANVASES-MARKETPLACE-002.4
  - AC-CANVASES-MARKETPLACE-003.3
  - AC-CANVASES-MARKETPLACE-003.4
  - AC-CANVASES-MARKETPLACE-005.1
  - AC-CANVASES-MARKETPLACE-005.3
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 07: Document canvas distribution

## Summary

Document the implemented package, sharing, installation, and registry workflow.
Keep public guides, registry instructions, and the embedded authoring reference
consistent with the shipped contract and UI helper.

## In scope

- Public canvas how-to: screenshot-free downloads, file/link/registry
  installation, permissions, independent copies, source modes, failure recovery.
- Plugin marketplace/manifest reference additions and registry README example.
  Previews belong to registry entries, not plugin or canvas package manifests.
- Manual GitHub repository/release/listing instructions; generic GitLab/Bitbucket
  bundle-hosting instructions that do not imply official registry support.
- Embedded authoring guide for retained source, screenshot-free distribution
  compatibility, and bounded files; preserve core inventory/read count.
- Scoped engineering-guide updates only where implementation changes conventions.
- Synchronize the new package's results/status and link related designs after
  each assigned check passes. Preserve older packages' recorded evidence.

## Out of scope

Publishing docs to another repository, submitting a registry entry, creating
a remote repository, adding generated promotional images, or a generic QA audit.

## Acceptance

- A reader can follow one complete manual export-to-repository-to-registry path,
  and can alternatively send a bundle file/direct URL without registry admission.
- Package examples match the production inspector, source and permissions.
  Registry examples show required canvas image URLs and optional plugin images,
  including custom sources; docs never require images for bundle creation.
- Public/embedded references, UI helper steps, and implemented contract agree;
  documentation validators and authoring inventory tests pass.

## Verification

```bash
rtk proxy node --test scripts/validate-public-docs.test.mjs
rtk proxy node scripts/validate-public-docs.mjs
(cd apps/backend && rtk go test ./internal/mcp/canvasskill ./cmd/canvas-package -count=1)
(cd apps/web && rtk pnpm exec vitest run components/settings/canvas-share-help.test.tsx)
rtk proxy python3 scripts/lint-spec-files.test.py
rtk proxy python3 scripts/lint-spec-files.py --all
rtk git diff --check -- docs plugin-registry/README.md apps/backend/internal/mcp/canvasskill
```

Validate documented package examples through inspector fixtures; do not publish
anything to prove documentation. Re-run UI E2E only if helper behavior or other
rendered UI changed in this task.

## Files likely touched

- `docs/public/canvases.md` (how-to)
- `docs/public/plugins-marketplace.md` (how-to)
- `docs/public/plugins-manifest.md` (reference)
- `docs/public/plugins-authoring.md` (linked canvas recipe only)
- `plugin-registry/README.md`
- `apps/backend/internal/mcp/canvasskill/files/SKILL.md`,
  `references/manifest.md`, `references/security.md`, inventory tests as needed
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md` if new conventions need recording
- `docs/specs/canvases/`, affected plugin design cross-links, this plan/work orders
- Audit root `README.md` and `docs/screenshots.md`; change only stale descriptions.

## Dependencies

Tasks 01-06 and their exact recorded results.

## Risks

Do not document proposed behavior as shipped before implementation. Existing
source download means the retained project, not a reconstruction of missing
build inputs. The official registry uses GitHub releases; host-neutral manual
sharing does not broaden that registry contract.

## Parallelism

`sequential`

## Inputs

- All six requirements and the final system design.
- /docs-maintainer and the public-document validation guide.
- Current registry README, public canvas/plugin pages, and UI-04 instructions.
- Existing canvas authoring core/scaffold inventory tests.

## Results

Updated the public canvas, plugin marketplace, manifest, and authoring guides;
the registry README; and the embedded authoring references. The documentation
keeps screenshots as registry metadata and documents screenshot-free sharing,
source modes, permissions, manual publication, and direct installation.

Verification: public-doc tests passed 62 tests and validated 46 published docs;
specification lint passed; the embedded authoring skill suite passed 7 tests;
and diff/whitespace checks passed.
