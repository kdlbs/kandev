---
id: "01-pua-font-fallback"
title: "Render Private Use Area codepoints via installed or bundled Nerd Font"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-paste-nerd-font-glyphs.md"
parallelism: sequential
---

# Task 01: PUA font fallback

## Context

`apps/packages/theme/src/fonts.css` contained no `@font-face` and no `unicode-range`
rule. `--font-sans` began `"Figtree", "Geist", ...`, none of which cover the
Private Use Area, so pasted Nerd Font glyphs rendered as notdef boxes.

Nerd Font family names already existed in `apps/web/lib/terminal/terminal-font.ts`
under the `icons` category, wired only to the terminal panel's
`terminalFontFamily` setting.

## Acceptance

- `NerdFontLocalGlyphs` covers all three PUA ranges and names every Nerd Font
  family by its full and PostScript aliases from the pinned v3.5.0 release.
- `NerdFontBundledGlyphs` uses only the content-hashed bundled URL and declares
  only ranges in the subset's actual character map.
- Bundled subset committed under `apps/web/public/fonts/nerd-symbols/` with its
  MIT licence, served from the app's own origin.
- Local and bundled families present in that order in every application font
  stack, after the UI typefaces.
- No text-processing code anywhere: bytes to the backend and agent unchanged.

## Verification

`cd apps/web && pnpm vitest run app/globals-font-fallback.test.ts`

Then `pnpm run typecheck` and `pnpm lint`.

Measured in a real browser against the running app: target codepoints render
the glyph rather than notdef; the bundled subset renders them with no `local()`
source present; ordinary text metrics are unchanged in an A/B with and without
the fallback.

## Files Likely Touched

- `apps/packages/theme/src/fonts.css`
- `apps/web/app/globals.css`
- `apps/web/app/globals-font-fallback.test.ts`
- `apps/web/public/fonts/nerd-symbols/nerd-symbols-subset-bca747e8.woff2`
- `apps/web/public/fonts/nerd-symbols/LICENSE`

## Output Contract

Report both faces, the declared ranges, the resolution order, the subset size
and coverage, tests run, and confirmation that no text-processing code was
introduced.
