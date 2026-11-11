---
id: "05-author-migration"
title: "Document author and maintainer migration"
status: done
wave: 5
depends_on:
  - 04-stable-publication
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
  - REQ-PLUGINS-SDK-DISTRIBUTION-004
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-002.1
  - AC-PLUGINS-SDK-DISTRIBUTION-002.3
  - AC-PLUGINS-SDK-DISTRIBUTION-003.4
  - AC-PLUGINS-SDK-DISTRIBUTION-004.2
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 05: Document author and maintainer migration

## Summary

Make the public author path use exact SDK versions and the CI compatibility
command. Align maintainer instructions and live source authority links with the
new module, while distinguishing implemented tooling from verified publication.

## In scope

- Update `docs/public/plugins-authoring.md` with installation/import migration,
  dependency pinning, removing replacements/file links, intentional upgrades,
  version/API/host mapping, and checker commands. Keep it a how-to/reference
  entry point and avoid adding another long duplicate contract.
- Update `docs/public/release-process.md` with npm ownership/bootstrap/OIDC,
  signing, SDK completion, and partial-publication recovery. Document retained
  asset/receipt locations, `bootstrap_required`, exact-archive manual upload,
  publisher setup, provenance limits, and retry verification. Explain recovery
  with an existing host tag but missing SDK tag, and unchanged-SDK reuse from
  its original release. Include dist-tag permissions and no-downgrade rules.
- Update root/backend guide references, the release/plugin-authoring skills,
  SDK README, and live specs/source links affected by the move. Inspect root
  README and screenshot catalog for stale SDK references. Preserve historical
  completed plan results; reconcile any still-pending work orders that reference
  removed paths without restarting those initiatives.
- Write an external template handoff in this plan package, naming `go.mod`,
  `go.sum`, `package.json`, `package-lock.json`, `.kandev-sdk-ref`, Makefile,
  README, source imports, and SDK-checkout workflow logic. Remove sibling
  requirements in that later repository change and run its Go tests/vet plus
  `npm run typecheck:recipes`, `npm run test:recipes`, and `npm run test:ui`.
  Mark that handoff as outstanding until the separate repository change and
  validation exist. Record its dependency on verified public package availability.
- Keep first host release/public install status pending until release evidence
  exists. Update this package's lifecycle/status only for verified implementation.

## Out of scope

Changing external repositories, creating platform follow-up tasks, publishing
the SDK, or treating example version placeholders as actual released mappings.

## Acceptance

1. Live author examples use the new import and pinned package flow, explain the
   SDK-wide floor and separate API axes, and include a runnable CI check.
2. Maintainer guidance distinguishes local implementation, registry bootstrap,
   automated provenance, release completion, and immutable recovery.
3. Live authority references and pending work-order paths resolve; a concrete
   template handoff and accurate design/plan lifecycle record are present.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
node scripts/pluginsdk/consumer-smoke.mjs --mode local
git diff --check
```

Also inspect `rg -n 'pkg/pluginsdk|apps/backend/proto|file:.*plugin-sdk|replace github.com/kandev/kandev'`
over root/scoped guides, public docs, SDK READMEs, applicable skills, and live
plugin designs. Remaining historical references must be identified as snapshots;
do not claim a global zero-match result while migration notes intentionally show
the old import.

## Files likely touched

- `docs/public/{plugins-authoring.md,release-process.md}`.
- Root `AGENTS.md`, `apps/backend/AGENTS.md`, relevant `.agents/skills` guides.
- `pluginsdk/README.md`, `apps/packages/plugin-sdk/README.md`.
- Live plugin SDK authority links in relevant requirements/designs and pending
  companion work orders.
- `docs/plans/plugin-sdk-publishing/{plan.md,task-*.md,template-migration.md}`.
- This package's requirement/design and ADR if implementation reveals a mismatch.

## Dependencies

[Task 04](task-04-stable-publication.md): final command names, release behavior,
and recovery contract.

## Risks

Public docs must not promise a registry version before its verified release.
Old completed plans contain intentional snapshots. External template adoption
remains a separate repository operation and must not be implied complete.

## Parallelism

`sequential`

## Author outreach preparation

Use the dated [plugin and author inventory](author-notification-inventory.md)
to prepare migration guidance for existing consumers. Refresh contacts before
outreach. Include both general and coordinator templates in the separate-repo
handoffs. Inspect backend-owned packaging tools before claiming checkout removal.
Notification delivery requires explicit authorization and is not part of this
work order. The inventory records candidates, not completed migrations.

## Inputs

- [Design: Author guidance](../../specs/plugins/system-design/sdk-distribution.md#author-guidance).
- Docs-maintainer and harness guidance; canonical template's current dependency
  files and its actual package scripts, verified during implementation.

## Results

Implemented on 2026-10-19. Public author guidance covers exact dependencies,
installed-version checks, SDK-wide host floors, and the existing-binary versus
source-build boundary. Release guidance covers protected setup, retained
artifacts, bootstrap provenance, public verification, and immutable recovery.
Root/backend guidance, release/plugin skills, live source authority links, and
both SDK READMEs use the extracted module. Historical completed records remain
snapshots, including earlier fixture test results that name the old SDK path.

The [template handoff](template-migration.md) records current commits in both
external templates and preserves their backend-owned packaging dependency.
Their migration and author outreach remain outstanding. The coordinator
package has no `test:ui` script at the inspected commit; the handoff records
that difference rather than claiming that command passed.

Validation: 63 public-doc validator tests passed; 48 public pages validate.
Harness and specification validator tests passed. All 204 harness files,
386 decisions, and 1626 specifications validate. README and screenshot catalog
inspection found no additional stale SDK import requiring edits.
Implementation CI and all review dispositions now pass. Cleanup validation also
passes 63 public-doc tests and 54 pages; see the plan for the final catalog counts.

## Completion

Completed on 2026-10-23 after local acceptance checks and implementation CI
passed. The [plan's cleanup record](plan.md#validation-and-review-cleanup)
contains the validation evidence and review dispositions. Public publication,
registry configuration, external repository migration, and author notices
remain separate follow-up work; completion here does not certify those actions.
