---
id: "07-capitalized-dynamic-labels"
title: "Capitalized dynamic Settings discovery labels"
status: completed
wave: 7
depends_on: ["06-browser-mobile-e2e"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 07: Capitalized dynamic Settings discovery labels

## Root cause

Dynamic workspace pages request nonexistent `common:repositories` and `common:workflows`
translations. i18next returns each raw key tail, so search results render lowercase even though
the Settings tree uses capitalized labels from the owning namespaces.

## Acceptance

- Dynamic repository and workflow results reuse the same translated labels as Settings navigation.
- A regression test fails on the raw-key fallback and passes after the key correction.
- Shared catalog resolution keeps desktop, phone, and Cmd+K output identical.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery/catalog.test.ts`

## Files

- `apps/web/lib/settings-discovery/resolve.ts`
- `apps/web/lib/settings-discovery/catalog.test.ts`

## Parallelism

Sequential; test and implementation change the same discovery contract.

## Results

- RED: focused catalog test received `repositories` and `workflows` instead of the capitalized
  navigation labels.
- GREEN: reused `sidebar:repositories` and `workflows:workflows`; 10/10 focused tests passed.
- Typecheck, focused ESLint, i18n checks, and the discovery translation-key audit passed.
- The isolated Tailscale test instance served the updated module and Settings route with HTTP 200.
- No separate mobile E2E was needed: this is shared catalog data normalization with no layout,
  touch, scrolling, navigation, or viewport-specific behavior change.
