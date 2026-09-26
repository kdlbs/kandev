# TYPO-001: Define a shared settings typography contract

- Priority: P0
- Status: Resolved
- Scope: All settings pages and shared settings primitives
- Related: TYPO-002, TYPO-003, TYPO-004, TYPO-005, TYPO-006

## Finding

Settings typography is currently an emergent result of base component defaults plus local Tailwind overrides. There is no settings-specific contract that names the expected family, size, weight, and line height for page titles, section headings, card titles, labels, helpers, controls, errors, and technical values.

This makes a page look different when it chooses a different primitive or when a developer forgets a local override. The desired outcome is a consistent semantic hierarchy, not a single size for every piece of copy.

## Evidence

- `apps/packages/ui/src/label.tsx:13` defaults labels to `text-xs/relaxed` and `font-medium`.
- `apps/packages/ui/src/input.tsx:11` uses `text-sm` with `md:text-xs/relaxed`.
- `apps/packages/ui/src/textarea.tsx:10` follows the same responsive `text-sm` to `md:text-xs/relaxed` pattern.
- `apps/packages/ui/src/select.tsx:40` uses `text-xs/relaxed` for the trigger, and `apps/packages/ui/src/select.tsx:114` uses the same scale for items.
- `apps/packages/ui/src/card.tsx:15`, `apps/packages/ui/src/card.tsx:37`, and `apps/packages/ui/src/card.tsx:44` set the card body, title, and description to separate defaults without a settings role layer.
- `apps/packages/ui/src/card.tsx:36-38` renders `CardTitle` as a non-heading `div`. Settings cards mix this default with manual nested `h3` elements, so visual type and screen-reader heading structure are both inconsistent.
- Existing E2E comments and assertions in `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts:24-35` and `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts:28-43` encode a local 12px/14px scale for one page, rather than a reusable semantic token.

## User impact

Equivalent settings content can change size and emphasis between pages. A contributor must inspect both the component default and the page-specific classes to predict the result. This is especially visible when moving between general preferences, agent profiles, workspace tabs, and system pages.

## Proposed direction

1. Agree on a semantic settings role map before changing individual pages. Preserve the existing h2 page and h3 section hierarchy while making card titles semantic headings where the card is a navigable content section.
2. Add the smallest shared implementation that can express those roles, preferably through existing settings primitives and semantic class constants rather than a new styling system. Keep locale keys resolved at render time so descriptions remain compatible with i18next and pseudo-locale checks.
3. Keep technical values separate from UI copy. Mono is appropriate for code, paths, identifiers, tokens, and logs, not as a general settings text style.
4. Migrate page shells and shared settings components first. Migrate feature-specific pages only after the shared roles exist.
5. Add a source-level rule or focused test for new settings typography classes so the scale does not drift again.

## Verification

- Render one representative page from each settings group at desktop and Pixel 5 widths.
- Measure computed styles for every role in a small typography fixture.
- Confirm translated and pseudo-locale labels still wrap without clipping.
- Update the existing notifications tests after the role values are finalized.

## Progress

- 2026-08-16: Opened from the settings source audit.
