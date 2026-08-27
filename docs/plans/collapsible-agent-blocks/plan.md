---
spec: docs/specs/agents/requirements/collapsible-agent-blocks.md
created: 2026-08-13
status: draft
---

# Implementation Plan: Collapsible Agent Blocks

## Overview

Frontend-only change to `/settings/agents`. A new domain hook persists a
per-agent collapse preference to `localStorage` (one JSON record keyed by agent
name, default all expanded, same storage-event + custom-event broadcast shape as
`useLocalStorageBoolean`). `InstalledAgentCard` wraps its profiles body in a
Radix `Collapsible` and, when collapsed, shows the profile count in the header.
Hooks and component get unit tests; the user-facing flow gets desktop and
mobile Playwright coverage.

Order: persistence hook first (task 01), then the UI that consumes it
(task 02), then E2E (task 03). Small feature: sequential, no waves.

---

## Backend

None. This feature is purely frontend (spec: "No backend, API, or database
change").

---

## Frontend

### Persistence hook (task 01)

New `apps/web/hooks/domains/settings/use-collapsed-agent-blocks.ts`:

- `STORAGE_KEY = "kandev:agents:collapsedBlocks:v1"` (JSON record
  `Record<string, boolean>`, `true` = collapsed).
- `SYNC_EVENT = "kandev:agents:collapsed-blocks-changed"`.
- `useSyncExternalStore` + `storage` event + custom-event broadcast, mirroring
  `apps/web/hooks/use-local-storage-boolean.ts` (read failures degrade to the
  default; write failures throw).
- API: `collapsed(agentName): boolean` and `setCollapsed(agentName, collapsed)`
  — the latter merges the entry into the stored record, persists it, and
  dispatches the sync event so other tabs/components re-render.
- Missing/empty record, missing entry, or invalid JSON → `false` (expanded).

### Component (task 02)

`apps/web/components/settings/installed-agent-card.tsx`:

- Wrap the card body in `Collapsible`/`CollapsibleTrigger`/`CollapsibleContent`
  from `@kandev/ui/collapsible`; `open` = `!collapsed`, `onOpenChange` calls
  `setCollapsed`.
- The trigger is a ghost icon button in the header actions cluster (before the
  runtime-update control), chevron icon rotating by state, touch-sized
  (`min-h-11 min-w-11`, mirroring `ProfileRowActions`), `cursor-pointer`, and
  `data-testid="collapse-agent-<name>"`. Its `aria-label` comes from new i18n
  keys `agents:collapseAgentProfiles` / `agents:expandAgentProfiles`
  ("Collapse {{name}} profiles" / "Expand {{name}} profiles").
- When collapsed, render the count in the header next to the trigger:
  `savedAgent.profiles.length === 0 ? t("agents:noProfilesYet") : t("agents:profileCount", { count })`.
  When expanded, the count stays where it is today (body first line of
  `AgentProfilesSubList`) and is NOT duplicated in the header.
- `children` (the `AgentProfilesSubList`) goes inside `CollapsibleContent`, so
  `data-testid="agent-profiles-<name>"` disappears when collapsed — the E2E
  assertion surface.
- `apps/web/app/settings/agents/page.tsx` needs no change (the sub-list is
  already passed as `children`).

### i18n (task 02)

- Add `collapseAgentProfiles` and `expandAgentProfiles` to
  `apps/web/src/locales/en/agents.json` (alphabetical placement).
- Regenerate the pseudo catalog: `cd apps/web && pnpm run i18n:pseudo`.
- Add translations to `pt-pt/agents.json` and `zh-cn/agents.json` (real-locale
  parity is advisory, but new keys should not increase drift).

---

## Tests

Every spec scenario maps to a check:

| Spec scenario | Test | File | How |
|---|---|---|---|
| Default expanded, nothing stored | hook returns `false` for unknown/missing entries; card renders children | hook test + component test | unit |
| Collapse hides list, count appears in header | toggle → `agent-profiles-<name>` hidden, "3 profiles" text in header | component test | unit |
| Reload keeps collapsed + count | record with `true` entry → card mounts collapsed | component test (pre-seeded `localStorage`) | unit |
| Expand restores list, header count gone | second toggle → body visible, count not in header | component test | unit |
| Per-agent independence | one entry does not affect another agent | hook test | unit |
| Zero profiles → "No profiles yet" in header | collapsed card with empty profiles | component test | unit |
| localStorage unavailable | `getItem` throws → default expanded; no crash | hook test | unit |
| Write failure throws | `setItem` throws → `setCollapsed` throws | hook test | unit |
| Full user flow, persistence across reload | desktop spec `agent-block-collapse.spec.ts` | `apps/web/e2e/tests/settings/` | Playwright |
| Same flow on mobile | mobile spec `mobile-agent-block-collapse.spec.ts` (auto-picked by `mobile-chrome` project) | `apps/web/e2e/tests/settings/` | Playwright |

## E2E Tests

- **Scenario (desktop)** `apps/web/e2e/tests/settings/agent-block-collapse.spec.ts`:
  GIVEN an installed agent with ≥1 profile, WHEN the user visits
  `/settings/agents`, THEN the profile body is visible; WHEN the user clicks
  the collapse control, THEN the body is hidden and the header shows the
  profile-count text; WHEN the user reloads, THEN the block is still collapsed;
  WHEN the user expands it, THEN the body is visible again. Uses `apiClient`
  (and creates a profile via `apiClient.createAgentProfile` when the fixture
  agent has none) so the count assertion is exact.
- **Scenario (mobile)** `apps/web/e2e/tests/settings/mobile-agent-block-collapse.spec.ts`:
  same user value on the `mobile-chrome` project: collapse a block, see the
  count in the header, reload, still collapsed. Touch target is the 44px
  collapse button; no desktop-only control is required.

## Verification Results

All three tasks completed. Exact commands and outcomes:

- Hook tests: `cd apps && pnpm --filter @kandev/web test -- hooks/domains/settings/use-collapsed-agent-blocks.test.ts` → 13/13 passed.
- Component tests: `cd apps && pnpm --filter @kandev/web test -- components/settings/installed-agent-card.test.tsx` → 6/6 passed.
- Typecheck: `cd apps/web && pnpm run typecheck` → clean; `make typecheck` → clean.
- Lint: `make lint` → clean (including `eslint --max-warnings 0` on all four new/changed files).
- i18n: `cd apps/web && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet` → all green.
- Format: `make fmt` → complete.
- E2E desktop: `cd apps/web && pnpm e2e:run -- tests/settings/agent-block-collapse.spec.ts` → 1 passed (2/2 with `--repeat-each=2`).
- E2E mobile: `cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-agent-block-collapse.spec.ts` → 3/3 with `--repeat-each=3`.
- `make test` (full): web suite 11303 passed; the only failures are environmental and pre-existing — `lib/http-git-server.test.ts` requires a Docker daemon, and six other web files fail only under full parallel load but pass in isolation. Backend: two packages fail for environment reasons unrelated to this frontend-only change (`internal/agent/settings/handlers` agent-update tests need a live npm registry; `internal/agentctl/server/api` needs the embedded VSCode remote-cli helper). This diff contains no Go files.

## Implementation Waves And Parallel Candidates

```
Wave 1 (sequential):
- [x] [task-01-collapse-persistence-hook](task-01-collapse-persistence-hook.md)
- [x] [task-02-collapsible-agent-card-ui](task-02-collapsible-agent-card-ui.md)
- [x] [task-03-collapse-e2e](task-03-collapse-e2e.md)
```

No parallel candidates: task 02 consumes task 01's hook, task 03 covers both.
E2E runs last (it needs the built app and both prior tasks).

## Open Questions

None.
