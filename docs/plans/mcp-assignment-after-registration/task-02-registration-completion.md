---
id: "02-registration-completion"
title: "Preserve successful MCP registration as a saved-only completion"
status: completed
wave: 2
depends_on:
  - "01-guard-selection-replacement"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
acceptance_criteria:
  - AC-AGENTS-MCP-ASSIGNMENT-001.1
  - AC-AGENTS-MCP-ASSIGNMENT-001.2
  - AC-AGENTS-MCP-ASSIGNMENT-001.3
  - AC-AGENTS-MCP-ASSIGNMENT-001.4
  - AC-AGENTS-MCP-ASSIGNMENT-003.4
  - AC-AGENTS-MCP-ASSIGNMENT-004.2
  - AC-AGENTS-MCP-ASSIGNMENT-004.3
  - AC-AGENTS-MCP-ASSIGNMENT-004.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
---

# Task 02: Saved registration completion

## Summary

Preserve the successful custom or marketplace create response and show a
saved-only completion in Configured. Isolate that success from later discovery
or navigation failures. Work order 03 adds the optional Use in control; do not
ship a disabled placeholder or claim the full assignment feature here.

## In scope

- Guard custom and marketplace mutation completion by workspace and request.
- Reconcile saved definitions by ID, close setup, select Configured, show Done.
- Support uncertain creation ACK reconciliation without automatic resubmission.
- Localized accessible feedback and focused saved-only desktop/phone E2E.

## Out of scope

Owner discovery, global-profile routing, selection writes, runtime actions, and
new durable receipt storage. Do not change definition edit behavior.

## Acceptance

1. Both creation paths show the same acknowledged definition identity in
   Configured; Done/dismiss/reload preserve it. Edit keeps current feedback.
2. Late workspace responses cannot pollute another catalog or open completion.
   Duplicate submission is blocked in flight; uncertain ACK recovery reads the
   catalog and does not issue another create or invent identity from a name.
3. Desktop/phone completion and localized status are reachable and do not call
   selection, runtime, package, endpoint, owner-create, or agent-start operations.

## UI preview

UI-01 from [full previews](plan.md#ui-01-saved-completion-both-viewports):

| Order | Desktop | Phone |
| --- | --- | --- |
| 1 | Saved to Acme / Atlas docs | Same saved identity, wrapping |
| 2 | Definition remains available in Configured | Same message in one column |
| 3 | Done | Touch-sized Done |

This work-order slice omits Use in until work order 03 makes it functional.
Setup closes first; no stacked overlay. UI-03's uncertain-create recovery uses
existing catalog review. Criteria: AC-001.1 through .4, AC-003.4, AC-004.2/.3/.4.

## Verification

Run TDD first. Work order 01 installs dependencies. From root:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-registration-completion.test.ts components/settings/workspaces/mcp-definition-form.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/mcp-assignment.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-mcp-assignment.spec.ts -- --retries=0
git diff --check
```

The completion hook/tests and assignment E2E files are proposed new files.
Include custom and marketplace in both projects, and assert no forbidden
requests. The managed runner builds fresh fixtures. Run commands sequentially.
Record discovered/executed tests and inspect a phone screenshot for wrapping.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx`
- `apps/web/components/settings/workspaces/mcp-marketplace.tsx`
- `apps/web/hooks/domains/workspace/use-mcp-workspace-settings.ts`
- New `mcp-registration-completion.tsx`, `use-mcp-registration-completion.ts`
  and tests; `e2e/helpers/mcp-assignment.ts`, two assignment specs.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/settings.json`
  and generated pseudo keys. Generate Traditional Chinese with `i18n:zh-hant`.

## Dependencies

Work order 01. No management sibling code is required for saved-only completion.

## Risks

Marketplace callback failure after create, unmount during save, duplicate local
catalog insertion, settings coordinator treating a saved receipt as dirty, and
misreporting uncertain registration as a confirmed success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-assignment.md)
- [Registration receipt](../../specs/agents/system-design/workspace-mcp-assignment.md#registration-receipt)
- Existing form contributor and workspace MCP E2E fixtures; `/tdd`, `/e2e`,
  `/mobile-parity`, and `better-ui` for the rendered result.

## Results

Implemented 2026-10-23. Custom and marketplace registration now share a safe
workspace/definition receipt in Configured. Done dismisses only the receipt.
Registration blocks duplicate in-flight requests, reconciles catalog IDs, and
rejects stale workspace generations. An uncertain response closes setup without
claiming success or advancing the settings save coordinator. Review saved servers
performs a catalog read before another registration is allowed. No name match is
treated as an acknowledged create. Definition edits keep their existing feedback.
The optional Use in control remains absent until work order 03 can be completed.

Verification:

- RED: six new registration tests reproduced duplicate submission/insertion,
  late workspace leakage, and missing uncertain-ACK recovery. A later test
  reproduced an old workspace's review error leaking into the new workspace.
- GREEN: all eight registration-state tests and the existing definition-form
  preservation test passed (9 tests). Web typecheck (8 GiB heap), changed-file
  ESLint with zero warnings, and all locale checks passed.
- Browser RED reached the missing saved receipt. Final managed desktop run:
  3 passed, zero retries; phone run: 3 passed, zero retries. Both include custom,
  marketplace, and persisted create followed by a deliberately lost response.
- E2E checks retain the exact catalog row across Done/reload, preserve repository
  membership, allow exactly one registration write, and reject all other writes
  except the existing sidebar's read-only POST query. No endpoint requests occur.
  Uncertain registration also asserts there is no false Saved settings bar.
- Phone screenshots for both successful paths were inspected. Long definition
  text wraps, Done fits the viewport and meets the 44px touch target, and there
  is no document horizontal overflow. Screenshots live in ignored E2E results.
- This Nix host required process-local Chromium library paths and
  `KANDEV_SERVER_HOST=127.0.0.1`. The inherited internal config-file/home-file and
  agentctl startup-config pointers were unset for fixture runs. Initial host
  binding and missing-browser-library failures did not reach the product tests.
  No runtime or shared machine configuration was changed.
- Public how-to guidance updated in `docs/public/automation-and-mcp.md`.
  Both public-doc validators passed. No screenshot catalog was changed.

Work order 01 commit: `f9137074309bebe99e2c7576baefe936bc20358f`.
Its parent was `193de56ba2597b335a97b1e5ab01d09340f22647`; both normal hooks
were active. Harness, docs catalog, architecture, specification, gofmt, Go lint,
Prettier, web lint, i18n, public-copy and commitlint passed. E2E sleep guard had
no files and was skipped. Bypass was false; worktree was clean after that commit.
Work order 02's commit receipt is recorded in the saved task plan after commit.
