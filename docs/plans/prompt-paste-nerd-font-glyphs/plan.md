---
spec: docs/specs/ui/requirements/prompt-paste-nerd-font-glyphs.md
created: 2026-08-19
status: done
---

# Implementation Plan: Render Nerd Font glyphs pasted from a styled terminal

## Overview
Map Private Use Area codepoints through two `unicode-range` scoped font faces.
The first face uses an installed Nerd Font. The second face uses a small subset
that Kandev ships. Separate families preserve per-character fallback when a
selected local font lacks one glyph. CSS and one font asset change. No text is
modified, so the bytes that reach the backend and the agent are unchanged.

## Confirmed root cause
The paste pipeline was never broken. TipTap, the WebSocket, SQLite and the
agent all carry the codepoints correctly; `U+E0B0` is transmitted as
`EE 82 B0`. The square is a render-time artifact.

Platform font-matching resolves missing glyphs for characters carrying a script
or language association (CJK, emoji), which is why those render unnamed.
Private Use codepoints carry none by definition, so the OS nominates no font
and the browser draws notdef. An explicitly named covering font is the only
mechanism that works.

`--font-sans` was `"Figtree", "Geist", ui-sans-serif, ...` with no `@font-face`
or `unicode-range` rule anywhere in the shared theme font catalog. Kandev's Nerd Font presets
in `lib/terminal/terminal-font.ts` were wired only to `terminalFontFamily` and
referenced by zero chat components.

## Decision record: why not sanitize
An earlier implementation on this branch (reverted) stripped the Private Use
codepoints at paste time. It was rejected on review because **the agent may
need those characters**: a user pasting a prompt theme or powerline config is
often asking a question *about* those glyphs, and removing them deletes the
subject of the question. Review also found the stripping approach silently
discarded glyphs inside fenced code blocks, exactly where literal content must
survive.

| Option | Font | Modifies sent text | User sees | Agent receives |
|---|---|---|---|---|
| 0 (before) | no | no | notdef box | unchanged |
| 1 (reverted) | no | yes, at paste | glyphs removed | modified |
| **2 (chosen)** | **yes** | **no** | **glyphs** | **unchanged** |
| 3 | yes | yes, at send | glyphs | modified |

## Two defects found during implementation
Both were discovered by testing in a real browser rather than by reading the
CSS, and both are silent failures.

1. **`local()` does not match family names.** The first implementation named
   families (`local("MesloLGS")`) and rendered nothing at all. `local()`
   matches a full font name or PostScript name only. Measured in Chromium
   against an installed font: `local("MesloLGS")` produces notdef,
   `local("MesloLGS Nerd Font Regular")` produces the glyph. An unmatched
   source falls through silently, so the rule looks correct and does nothing.
2. **Ordering decided the winner arbitrarily.** Pasting the catalogue in
   alphabetical order handed the match to `MesloLGL` over `MesloLGS` purely on
   sort position. Harmless there (identical outlines) but wrong in general,
   since glyph shapes differ between families and the closest match to the
   user's terminal is preferred.

## Approach
1. `NerdFontLocalGlyphs` in `apps/packages/theme/src/fonts.css` covers all
   three PUA ranges. Its `src` lists the full and PostScript aliases for the 66
   Nerd Font families in the v3.5.0 release, ordered by intent.
2. `NerdFontBundledGlyphs` contains only the bundled URL. Its `unicode-range`
   matches the subset's actual character map, so unsupported PUA codepoints do
   not cause a download that cannot render them.
3. Bundled subset generated from the MIT-licensed `SymbolsNerdFont-Regular`
   v3.5.0 with `fontTools`, restricted to the four icon sets a prompt uses, and
   committed with its licence and a content-hashed filename at
   `apps/web/public/fonts/nerd-symbols/`.
4. Both families carry `size-adjust: 75%`. Every font stack puts the local face
   before the bundled face and puts both after the UI typefaces.
5. A guard test covers source separation, range accuracy, cache-safe naming,
   and stack order.

### Subset size curve (measured, woff2)

| Ranges | Size |
|---|---|
| powerline only | 7 KB |
| + octicons | 33 KB |
| + seti/custom folders | 71 KB |
| **+ devicons (chosen)** | **236 KB** |
| + codicons | 280 KB |
| + Font Awesome | 472 KB |
| entire BMP PUA | 583 KB |

Font Awesome doubles the payload for icons prompts rarely draw, so the cut is
after devicons. The bundled face declares its exact character map, so it is
fetched only when a codepoint that the file supports is rendered.

## Tasks
- `task-01-pua-font-fallback.md` — shared theme font catalog, bundled subset,
  guard test

## Validation
From `apps/web`:
```bash
pnpm vitest run app/globals-font-fallback.test.ts
pnpm run typecheck
pnpm lint
```

Measured against the running app rather than asserted:
- `U+E0B0`, `U+F418`, `U+E5FF` render the installed glyph, not notdef.
- Winner is `MesloLGS Nerd Font Regular`, matching the terminal's own font.
- Bundled subset alone (no `local()`) renders powerline, git branch, folder and
  devicon glyphs, proving the path for users with no Nerd Font.
- Separator drops from 1.99x to ~1.43x cap height under `size-adjust`.
- A/B of the stack with and without the fallback leaves Latin, accented, CJK,
  emoji and box-drawing widths byte-identical.

## Risks
- A Nerd Font outside the catalogue falls through to the bundled subset, so
  common glyphs still render but family-specific ones may not.
- The first installed local source still wins. If it lacks one glyph, the
  separate bundled family can render that character. The browser cannot try a
  second installed source from the same `src` list for that character.
- Glyphs outside the subset (plane-15 Material Design, Font Awesome) remain
  notdef for users with no Nerd Font. Widening requires regenerating the
  subset from the pinned source archive and updating the CSS coverage and
  recorded output hash. The measured size cost is tabulated above.
- CSS has no unit-testable logic, so the guard asserts the declaration's shape.
  Rendering was verified by measuring glyph advance widths in a real browser.

## Reproducible subset source

The bundled file uses `NerdFontsSymbolsOnly.zip` from Nerd Fonts v3.5.0. The
source archive SHA-256 is
`49362450cd61b32c7d1dadbb98e82696d77cc215344636d25eabc8a82d6f8d7f`.

After extracting `SymbolsNerdFont-Regular.ttf`, regenerate the file with:

```bash
python -m fontTools.subset SymbolsNerdFont-Regular.ttf \
  --unicodes="U+E0A0-E0D4,U+F400-F533,U+E5FA-E6B7,U+E700-E8EF" \
  --flavor=woff2 \
  --output-file=apps/web/public/fonts/nerd-symbols/nerd-symbols-subset-bca747e8.woff2
```

The committed WOFF2 SHA-256 is
`bca747e8daab16b628ebbc40bf67cd3ac961c143a40f794b9951c3f4e31e0618`.
The filename uses the first eight hash characters because static assets have a
one-year immutable cache policy.
