---
id: "04-editor-reconciliation"
title: "Reconcile live profile edits"
status: done
wave: 4
depends_on:
  - "03-settings-discovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-004
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
---

# Task 04: Reconcile live profile edits

## Summary

Show assistant changes in an open profile editor while preserving unfinished user edits.
Prove the result through desktop and mobile flows, with independent frontend coverage checks.

## In scope

- Reconcile authoritative store updates with clean, dirty, submitted, and conflicted editor states.
- Distinguish the editor's own acknowledgement from an external update.
- Cover concrete drafts, dynamic candidate drafts, and the separate MCP-document contributor.
- Refetch MCP documents after value-free invalidation, with profile-ID and request-generation guards.
- Block stale saves through the existing contributor and use its discard/reset flow for recovery.
- Add localized inline conflict copy and preserve current responsive composition.
- Connect profile UI patch keys and MCP editor fields to the generated metadata snapshot.
- Add real-MCP browser scenarios and the relevant contract/component tests.
- Drive new browser scenarios through compact read/update tools, including the separate MCP-document resource type.
- Document live results and draft recovery in the profile guide.

## Out of scope

- A new global editor, navigation redesign, or backend optimistic locking.
- Automatic merging of competing profile changes.
- New standalone Save/Cancel controls.

## Acceptance

- Clean editors adopt assistant changes and retain them after reload. Dirty drafts survive and require baseline recovery before Save.
- Own-save acknowledgements and late responses do not erase newer edits or create false conflicts.
- Desktop and mobile complete the same flow. Coverage fails for an unmapped frontend profile field, and new copy passes localization checks.

## Verification

On a fresh worktree, install dependencies once before the frontend commands.
Run all commands from the repository root.

```bash
rtk pnpm --dir apps install --frozen-lockfile
rtk pnpm --dir apps/web exec vitest run components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-page.test.tsx components/settings/dynamic-agent-profile-editor-state.test.tsx lib/settings-discovery/profile-contract.test.ts lib/ws/handlers/agents.test.ts 'app/settings/agents/[agentId]/use-profile-mcp-config.test.ts'
rtk pnpm --dir apps/web run typecheck
rtk pnpm --dir apps/web run i18n:check
rtk pnpm --dir apps/web e2e:run --project chromium tests/settings/agent-settings-parity.spec.ts
rtk pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-agent-settings-parity.spec.ts
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

The managed E2E runner rebuilds production assets and isolates the backend.
Run desktop and mobile commands sequentially. Record test discovery counts and rendered phone evidence.
The shared scenario helper uses the real MCP transport and disposable profiles, not raw gateway actions.
Use `tap()` for phone interactions and causal event waits for mutations.

## Files likely touched

- `apps/web/components/settings/agent-profile-page-state.ts` and tests.
- `apps/web/components/settings/agent-profile-page.tsx` and tests.
- `apps/web/components/settings/dynamic-agent-profile-editor-state.ts` and draft helpers with focused tests.
- `apps/web/app/settings/agents/[agentId]/use-profile-mcp-config.ts` and tests.
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx`.
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.ts`.
- `apps/web/lib/ws/handlers/agents.ts` and tests, only if reconciliation requires a store change.
- `apps/web/lib/settings-discovery/profile-contract.ts` and tests (new).
- `apps/web/lib/settings-discovery/` profile target mapping and existing domain patch types.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/agents.json`.
- `apps/web/e2e/tests/settings/agent-settings-parity.spec.ts` (new).
- `apps/web/e2e/tests/settings/mobile-agent-settings-parity.spec.ts` (new).
- `apps/web/e2e/tests/settings/agent-settings-parity-helpers.ts` (new).
- `docs/public/agents-and-profiles.md`.

## Dependencies

Tasks 01 through 03 supply metadata, profile mutations, and the real MCP read/write path.

## Risks

- A notification can arrive before the editor receives its own save response.
- The selected profile can change while a request is pending.
- A delayed response can be older than the latest WebSocket snapshot.
- MCP document invalidations can arrive during a refetch. A stale read must not become the new baseline.
- Shared seed profiles can leak state between E2E tests. Use disposable profiles and cleanup after failures.

## Mobile contract

Reuse the existing profile route and advanced sections from `mobile-agent-profile-config-selector.spec.ts`.
Keep the existing content scroll owner and shared save surface.
Show conflict information inline. The existing discard/reset action supplies recovery.
Preserve touch reachability, keyboard access, safe-area clearance, and zero document horizontal overflow.
No new overlay or breakpoint branch is required.

## Parallelism

`sequential`

## Inputs

- System design: Editor reconciliation, Coverage.
- Existing profile state, profile selector E2E, and save-coordinator contracts.
- `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.

## Results

Completed on 2026-09-09.

- Added profile editor reconciliation for clean, dirty, own-save, and late-response cases.
- Added MCP profile invalidation handling with guarded refetch and localized conflict recovery.
- Preserved dirty drafts on external changes and covered desktop/mobile settings transport.
