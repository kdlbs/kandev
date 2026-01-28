# Web i18n Implementation Plan

**Status**: Draft for execution
**Scope**: `apps/web` (Next.js) only
**Owner**: TBD
**Last updated**: 2026-01-28

## 1) Goals

- Provide first-class locale support in the web app with SSR-friendly message loading.
- Preserve the existing data flow pattern: **SSR fetch → hydrate store → components read store → hooks subscribe**.
- Enable safe, incremental migration from hardcoded strings to message keys.
- Provide clear contributor workflow for adding strings and new locales.

## 2) Non-goals

- No backend or landing site i18n in this phase.
- No runtime translation or machine translation in production.
- No in-component data fetching or ad hoc localization in components.

## 3) Assumptions (confirm before implementation)

- **Locales**: `en` (default) and `es` (secondary). Additional locales can be added post-v1.
- **Translation source**: Repo-local JSON catalogs: `apps/web/messages/{locale}.json`.
- **Routing**: Locale-prefixed routes (`/en/...`, `/es/...`). If default-locale prefix is not desired, adjust middleware strategy.
- **Router**: Next.js App Router. If Pages Router is in use, adapt implementation accordingly.

## 4) Architecture Overview

### 4.1 Library choice

- **Preferred**: `next-intl` (App Router friendly, SSR support, message loading, formatter helpers).
- **Alternative**: `react-intl` (more manual routing + loading; only if `next-intl` conflicts with architecture).

### 4.2 Locale resolution

- **Order**: route prefix → cookie → `Accept-Language` header → fallback (`en`).
- **Middleware** enforces locale prefix and sets cookie for future requests.

### 4.3 Message loading

- Messages loaded **server-side** in root layout for each locale.
- Messages passed to provider and available to server components and client components.
- No direct message fetching from components.

### 4.4 State hydration alignment

- Store includes `ui.locale`, `ui.localeSource` (route/cookie/header), and `ui.timeZone` if needed.
- SSR hydration must not override active sessions; use existing merge strategies in `lib/state/hydration`.

### 4.5 Formatting

- Use locale-aware formatting helpers (dates, numbers, relative time). No `toLocaleString` direct usage in components.

## 5) Proposed Project Structure

```
apps/web/
  messages/
    en.json
    es.json
  middleware.ts
  app/
    [locale]/
      layout.tsx
      page.tsx
      ...
  lib/i18n/
    config.ts
    routing.ts
    messages.ts
    formats.ts
    types.ts
```

## 6) Implementation Phases

### Phase 0 — Audit and alignment

- Inventory current string sources (components, hooks, stores, constants).
- Identify locale-sensitive formatting (dates, numbers, relative timestamps).
- Confirm App Router and SSR entry points.

### Phase 1 — Core i18n plumbing

- Add `messages/{locale}.json` catalogs with minimal base keys.
- Introduce `lib/i18n/config.ts` for locales, default locale, and fallback policy.
- Add middleware for locale detection and prefix enforcement.
- Create `[locale]/layout.tsx` with provider setup and message load.
- Create typed message helper for key safety (optional first pass).

### Phase 2 — Hydration and store integration

- Extend UI slice with locale/timezone state.
- SSR pipeline injects locale into store hydration; ensure it does not clobber live state.
- Add `useLocale()` hook or selector wrapper to keep components lean.

### Phase 3 — Vertical slice migration

- Migrate a single page and global layout strings (nav, sidebars, empty states).
- Add date/number formatting wrappers for UI pieces in that slice.
- Validate SSR + hydration behavior in dev and test environment.

### Phase 4 — Scale out migration

- Convert remaining UI strings, grouped by feature (kanban, sessions, settings, workspace).
- Add lint rule or script for missing keys (basic report).
- Add CI check to prevent missing keys for existing locales.

### Phase 5 — Documentation and workflow

- Document how to add keys and new locales.
- Document routing behavior and locale debugging.
- Provide a small internal glossary for common UI terms.

## 7) Detailed Task List (initial)

- [ ] Confirm locales, routing, and translation ownership.
- [ ] Add `messages` directory and seed `en.json`.
- [ ] Add `lib/i18n/config.ts` with supported locales and default.
- [ ] Add middleware to enforce `/[locale]` routing and set locale cookie.
- [ ] Create `app/[locale]/layout.tsx` provider setup.
- [ ] Implement `lib/i18n/messages.ts` loader.
- [ ] Add locale to store hydration path.
- [ ] Add formatting helpers for dates/numbers/relative time.
- [ ] Migrate layout, top nav, and one page end-to-end.
- [ ] Add build-time check for missing translations.
- [ ] Document contribution workflow.

## 8) Risks & Mitigations

- **Risk**: Locale prefix changes existing routes.  
  **Mitigation**: Add redirects in middleware and update internal links.

- **Risk**: SSR hydration overwrites live session state.  
  **Mitigation**: Use existing merge strategies and only merge locale keys.

- **Risk**: Format inconsistencies for dates/numbers.  
  **Mitigation**: Centralize formatting helpers and ban direct `toLocaleString` usage.

- **Risk**: Untranslated strings degrade UX.  
  **Mitigation**: Default fallback and CI translation checks.

## 9) Testing Strategy

- Unit tests for locale resolution and message loading.
- Render test for one localized page with `en` and `es`.
- Manual smoke test: locale switching, routing redirects, SSR render.
- Lint/check script for missing keys or unused keys.

## 10) Rollout Plan

- Ship core plumbing and one localized slice behind a feature flag if needed.
- Expand to full UI once stable in staging.
- Add new locales only after all `en` keys are migrated and tooling is in place.

## 11) Acceptance Criteria

- Locale-specific routes render correctly with SSR.
- All critical UI strings are localized in `en` and `es`.
- No in-component fetches added for i18n.
- CI check prevents missing keys for supported locales.
- Documented workflow for adding strings/locales.

---

If you want a different scope (non-prefixed default locale, different library, more locales, or external translation source), I will revise this plan accordingly.

## Appendix A) Workspace Listing (ls -l)

```
$(ls -l)
```
