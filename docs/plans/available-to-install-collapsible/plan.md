---
spec: docs/specs/ui/available-to-install-collapsible.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Available to Install section collapsible

## Overview

Wrap the "Available to Install" section on the Agents settings page in the existing Radix-based `@kandev/ui/collapsible` primitive: the heading row becomes the trigger (button semantics, rotating chevron) and the card grid moves into `CollapsibleContent`. Local `useState` keeps the section expanded by default and resets on navigation. No backend, copy, or install-flow changes. Desktop and mobile E2E prove the collapse/expand behavior.

## Frontend

### Agents settings page

- `apps/web/app/settings/agents/page.tsx`, `SuggestInstallSection`: convert the static header `<div>` (title + description) plus card grid into:

  ```tsx
  const [open, setOpen] = useState(true);
  ...
  <Collapsible open={open} onOpenChange={setOpen}>
    <CollapsibleTrigger asChild>
      <button
        type="button"
        className="flex w-full items-start justify-between gap-3 cursor-pointer"
        data-testid="available-to-install-trigger"
      >
        <div className="text-left">
          <h3 className="text-lg font-semibold">{t("agents:availableToInstall")}</h3>
          <p className="text-sm text-muted-foreground">{t("agents:availableToInstallDescription")}</p>
        </div>
        <IconChevronDown
          className={`h-5 w-5 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
        {/* existing InstallCard + ToolInstallCard map */}
      </div>
    </CollapsibleContent>
  </Collapsible>
  ```

- Add `IconChevronDown` to the existing `@tabler/icons-react` import and `Collapsible, CollapsibleContent, CollapsibleTrigger` from `@kandev/ui/collapsible`. `useState` is already imported.
- Keep the `Separator` above the heading, the `space-y-4` wrapper, and the `if (notInstalledAgents.length === 0 && notInstalledTools.length === 0) return null;` guard.
- Radix sets `aria-expanded` / `data-state` on the trigger automatically; the accessible name comes from the heading text, so no new i18n strings are needed.

### i18n

- No locale changes: existing `agents:availableToInstall` and `agents:availableToInstallDescription` strings are reused on the changed lines; `pnpm run i18n:ratchet` must stay green.

## Mobile design contract

- **Desktop outcome:** the section's card grid can be hidden and shown via the heading row.
- **Mobile entry point:** the existing Settings navigation to the Agents page; the heading row itself is the touch control.
- **Nearest exemplar:** `apps/web/components/app-sidebar/app-sidebar-section.tsx` (header-trigger collapsible with rotating chevron) and the office run-detail collapsibles (`invocation-panel.tsx`, `prompt-panel.tsx`) that use the same `@kandev/ui/collapsible` + chevron pattern.
- **Hierarchy and action:** the heading row is the primary toggle; no new drawer, overlay, or navigation surface.
- **Surface rationale:** content depth is a flat grid of cards; an inline collapse matches the task frequency better than a drawer.
- **Scroll and touch:** the section keeps document scrolling; the full heading row is the touch target (heading + description exceed 44 px together), the chevron is right-aligned and shrink-safe; no fixed or absolute positioning is introduced, so no document horizontal overflow risk.
- **Shared versus responsive behavior:** the same component and local state serve both viewports; no responsive branching.

## Tests

- **What:** collapse/expand toggling of the section's card grid.
  - **File:** `apps/web/e2e/tests/settings/available-to-install-collapsible.spec.ts`
  - **How:** Playwright E2E against the seeded mock backend; assert grid card visibility, trigger `aria-expanded`, and re-expansion.
- No unit test: the change is presentational component markup (an explicit test exception in `apps/web/AGENTS.md`), and the page component has no existing unit harness.

## E2E Tests

- **Scenario (desktop):** GIVEN the Agents settings page with an installable agent, WHEN the user clicks the "Available to Install" heading row, THEN the card grid hides (`aria-expanded` false) and clicking again restores it.
  - **File:** `apps/web/e2e/tests/settings/available-to-install-collapsible.spec.ts`
  - **What to verify:** `data-testid="available-to-install-trigger"` toggles `aria-expanded`; `install-card-<agent>` is hidden when collapsed and visible when expanded.
- **Scenario (mobile):** the same value at a phone viewport via tap, with no document horizontal overflow.
  - **File:** `apps/web/e2e/tests/settings/mobile-available-to-install-collapsible.spec.ts` (auto-picked by the `mobile-chrome` Playwright project).

## Implementation Waves

Wave 1:

- [x] [Task 01: Make the section collapsible](task-01-collapsible-section.md)

Wave 2:

- [x] [Task 02: Prove desktop and mobile collapse flows](task-02-e2e-coverage.md)

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` → done (needed for the commit-msg hook's `commitlint`; 6s).
- `cd apps/web && pnpm run typecheck` → clean (131s).
- `cd apps/web && pnpm run i18n:ratchet` → `✓ i18n new-code ratchet — 0 added + 1 modified file(s) clean`.
- `make fmt` → `✓ Code formatting complete!`; `make lint-web` → clean (eslint `--max-warnings 0`).
- `cd apps/web && pnpm e2e:run --host tests/settings/available-to-install-collapsible.spec.ts` → `1 passed (6.6s)` (chromium project; red on the pre-implementation build, green after).
- `cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-available-to-install-collapsible.spec.ts` → `1 passed (6.8s)` (mobile-chrome project).
- `make test-web` → 10495 passed, 4 skipped; 3 remaining failures are `lib/http-git-server.test.ts` tests that require a Docker daemon bridge gateway unavailable in this sandbox — they fail in fixture setup on the base tree and are unrelated to this change. All other initially failing tests (file-browser, sentry, lazy-catalogs, error-boundary) pass in isolation (parallel-load timeouts).

## Risks

- The E2E fixtures must seed a discoverable-but-not-installed agent so the section renders; `agent-install-streaming.spec.ts` and the mock-agent `InstallScript` fixtures already provide this pattern.
- Radix unmounts `CollapsibleContent` when closed (it is not merely visually hidden): tests must assert `toBeHidden()`, which covers both detached and hidden states.
- The heading text lives inside the trigger button, so `getByRole("button", { name: /Available to Install/ })` and a heading-role query both resolve to the trigger; tests should target the trigger by testid.
