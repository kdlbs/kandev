---
id: "02-initial-owner-publication"
title: "Authorize initial owner publication"
status: done
wave: 2
depends_on:
  - "01-same-origin-embedding"
plan: "plan.md"
requirements:
  - REQ-CANVASES-LOCAL-CREATION-001
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-003
acceptance_criteria:
  - AC-CANVASES-LOCAL-CREATION-001.1
  - AC-CANVASES-LOCAL-CREATION-001.2
  - AC-CANVASES-LOCAL-CREATION-001.3
  - AC-CANVASES-LOCAL-CREATION-001.4
  - AC-CANVASES-LOCAL-CREATION-001.5
  - AC-CANVASES-LOCAL-CREATION-001.6
  - AC-CANVASES-LOCAL-CREATION-001.7
  - AC-CANVASES-AGENT-WEB-APPS-001.2
  - AC-CANVASES-AGENT-WEB-APPS-003.4
  - AC-CANVASES-AGENT-WEB-APPS-003.5
system_design:
  - ../../specs/canvases/system-design/local-creation-authority.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 02: Authorize initial owner publication

## Summary

Record trusted creation authority for new owner-created drafts. Consume it in
the first publication transaction, so a validated canvas opens without a second
approval while later expansions still use review.

## In scope

- Canvas-owned authority table, atomic creation, cleanup, and schema replay.
- Strict owner resolution in the trusted authoring adapter; no error fallback
  as authority and no manifest/tool-supplied trust flag.
- Full authority snapshot validation and exact initial grants in the existing
  instance publication transaction, including normalized HTTPS origins.
- Additive create-response policy description and bundled guidance update.
- Separate owner-created and legacy/manual fixtures, plus desktop/mobile first
  publication and subsequent-review coverage. Preserve promotion and revocation.
- Public creation/security guidance and relevant backend scoped guidance.

## Out of scope

Imports/marketplace UI, collaborator identity redesign, retroactive grants,
later automatic increases, startup/review layout changes, and live-task repair.

## Acceptance

1. New `TestCanvasCreationAuthorityFirstPublish` fails on the current manual
   gate, then proves exact persisted grants and activation with no approval.
   Zero-permission first publication consumes authority as well.
2. Foreign/missing owner, wrong session/task, installed/imported source,
   legacy drafts, unsupported capabilities, later increases, and revoked
   grants cannot use the initial exception. Include mixed eligible/ineligible
   instances. Normal no-new-permission edits and manual review still work.
3. Concurrent publish, owner/scope/grant drift, and injected transaction failure
   leave no partial grants or reusable authority. Restart, cleanup, and both
   database dialects preserve the recorded policy.

## ASCII UI preview

Creation uses the existing task panel and phone route; no new creation form.
Excerpt of [UI-01 in the plan](plan.md#ui-01-canvas-startup-task-panel-or-focused-route):

```text
Desktop task: [Agent publishes] -> [Live workflow tab opens]
Phone task:   [Agent publishes] -> [< Back | Live workflow]
Both:         [Loading...] -> [Application]
              No initial permission dialog
Later increase: [Current app] -> [Review pending permissions]
```

At this work order, assert actual frame content rather than relying on the
pre-existing Ready label. Task 03 establishes that label's final semantics.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/canvas/... ./internal/plugins/instances/...)
(cd apps/backend && go test ./internal/backendapp -run 'Canvas' -count=1)
(cd apps/backend && go test ./internal/mcp/canvasskill ./internal/mcp/handlers -run 'Canvas|Bundle|Scaffold' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use current database replay conventions for the new table. Add authority tests
to the selected packages and use Canvas-prefixed backendapp regression names.
Document any unavailable PostgreSQL service rather than claiming dialect proof
from SQLite alone. Preserve `TestPublishPackageFirstReleaseRequiresMatchingGrants`
for the no-authority case. Do not turn every fixture into an auto-approved one.

## Files likely touched

- `apps/backend/internal/canvas/{repository.go,types.go,service.go,authoring.go,canvas_test.go,authoring_test.go}`
- `apps/backend/internal/plugins/instances/{store.go,store_test.go,store_cleanup_test.go}`
- `apps/backend/internal/backendapp/{canvas_authoring.go,services_canvas_test.go,canvas_authoring_scaffold_test.go}`
- `apps/backend/internal/mcp/canvasskill/{bundled.go,bundled_test.go,files/SKILL.md,files/references/manifest.md,files/references/security.md}`
- `apps/backend/internal/mcp/handlers/canvas_test.go`
- `apps/web/e2e/tests/canvas/{canvas-fixture.ts,plugin-canvas.spec.ts,mobile-plugin-canvas.spec.ts}`
- `docs/public/{canvases.md,security.md,plugins.md,plugins-authoring.md}`
- `apps/backend/AGENTS.md` for the durable creation-authority boundary

## Dependencies

Task 01 permits faithful runtime browser verification. Implement all schema,
adapter, service, and fixture changes in this single sequential work order.

## Risks

Creation owner lookup is authorization, not display attribution. Reusing its
current fallback would be unsafe. Initial declared writes/network access are
authorized deliberately; later grant expansion must remain separate.

## Parallelism

`sequential`

## Inputs

- [Creation design](../../specs/canvases/system-design/local-creation-authority.md).
- [Creation decision](../../decisions/2026-09-10-canvas-creation-authority.md).
- `persistAuthorityRelease`, `CreateReleaseIfAuthorityTx`, grant normalization,
  and existing approval/retention transaction tests.

## Results

Implemented and verified.

- Added recorded, single-use creation authority for owner-created local canvas
  drafts. The first eligible publication consumes the authority and atomically
  creates the exact task-scoped grants, assigns the approving owner, and
  activates the release. Existing drafts, imports, source/task/session
  mismatches, unsupported manifests, and later permission increases remain on
  the review path.
- Made owner resolution fail closed in the authoring adapter and returned the
  additive initial permission policy in the create response. Updated public and
  bundled authoring/security guidance and E2E fixtures to distinguish trusted
  first publication from manual approval.
- Added rollback and cleanup coverage for authority, release, identity, and
  grant state, plus backend and database-store regressions.
- Targeted verification passed: Canvas/instance Go packages (49 tests),
  Canvas backendapp tests (23), canvas MCP/handler tests (12), desktop Canvas
  E2E (2), mobile Canvas E2E (3), public-doc tests (62), public-doc validator
  (46 pages), specification lint, and `git diff --check`.
