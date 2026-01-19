# Initial State Hydration Refactor (LLD)

## Goal
Eliminate per-component `initial*` props by creating the Zustand store with preloaded data at the layout/route boundary, so SSR content renders immediately without post-hydration flashes.

## Current Behavior
- `StateProvider` is in `app/layout.tsx` and always instantiates the store without initial data.
- `StateHydrator` injects server data via `useEffect`, which runs after first paint.
- Pages pass `initial*` props to avoid empty SSR output.

## Proposed Architecture (Settings)
### 1) Introduce a Settings Store Provider
Create layout-level client providers that accept `initialState` and create the store once:
- `apps/web/components/state-provider.tsx`
  - keep existing `StateProvider` API but allow `initialState` to be passed.
- new `SettingsStateProvider` (or reuse `StateProvider` directly in settings layout) that accepts `initialState` and wraps settings pages.

### 2) Move Store Creation From Root Layout (Settings Only)
- Root `app/layout.tsx` should no longer create the store (or should accept and pass an initial state when available).
- For sections that need SSR data, create the store in their layout and pass `initialState`.

### 3) Replace StateHydrator With Initial Store Creation
- For sections using SSR data, remove `StateHydrator` from those layouts/pages.
- Create the store with `createAppStore(initialState)` at layout render time.

### 4) Update Settings Layout (Fix Sidebar Flash)
- `apps/web/app/settings/layout.tsx`
  - Build `initialState` exactly as today.
  - Wrap children with `<StateProvider initialState={initialState}>` (or new `SettingsStateProvider`).
  - Remove `<StateHydrator>` usage.
  - **Sidebar fix:** The sidebar reads from `settingsAgents`, `workspaces`, etc. With a settings-level provider created from server data, the sidebar will render populated on first paint (no post-load flash).

### 5) Update Pages Under Settings
- Remove `initial*` props that were only used to avoid flash.
- Components should read from store only (via hooks) and use store `loaded` flags.
- Example: `PromptsSettings` should read prompts from store, no `initialPrompts` prop.

### 6) Decide Provider Boundaries
Two options:
- **Option A (Scoped provider):** Settings layout creates its own store instance. Pros: isolates settings state. Cons: state is not shared with other routes.
- **Option B (Shared store):** Root provider accepts initial state per route (requires root layout to receive data, which Next.js does not support directly). Practically hard with App Router.

**Recommendation:** Option A (Scoped provider) per section (e.g., Settings), so SSR data is injected at first render without a flash.

## Detailed Steps
1) **State Provider Changes**
   - Update `StateProvider` to accept `initialState?: Partial<AppState>` (already in types).
   - Ensure it uses `useState(() => createAppStore(initialState))` (already does).
2) **Settings Layout**
   - Replace `StateHydrator` with `StateProvider` wrapping `SettingsLayoutClient`.
   - Remove provider from root layout **only if** it conflicts. Otherwise, nest a provider for settings only.
3) **Settings Layout Client**
   - If nested provider is used, ensure `SettingsLayoutClient` uses the nearest provider.
4) **Pages/Components Cleanup**
   - Remove `initial*` props (e.g., `initialPrompts`, `initialEditors`, etc.).
   - Update hooks/components to rely on store state.
5) **Hooks**
   - Keep hooks as-is but remove special-casing for initial props.
6) **SSR Validation**
   - Verify that settings pages render with data on first paint.
   - Remove `StateHydrator` usages where the new provider is used.

## Migration Plan (Settings Focus)
1) Implement provider shift for Settings only.
2) Remove `StateHydrator` from `apps/web/app/settings/layout.tsx` and create the store with `initialState`.
3) Ensure Settings sidebar uses the settings-level store (no data flash).
4) Update settings pages (prompts/editors/notifications) to remove initial props if desired.
5) Add regression checks for settings pages to ensure no blank state on first render.
6) Optionally expand to other sections later.

## Risks
- Nested providers create isolated state between main app and settings pages.
- Multiple store instances may impact shared data expectations (e.g., websockets updating a different store).
- Must ensure WS connector uses the correct store instance or remains in root if necessary.

## Acceptance Criteria
- Settings pages render data immediately without post-load flash.
- Components do not require `initial*` props to render lists.
- No runtime errors due to missing provider context.
