---
status: active
system: ui
created: 2026-08-19
owners:
  - jnmanso
---
# Render Nerd Font glyphs pasted from a styled terminal Requirements

## Overview

Text copied from a styled terminal prompt (Oh My Posh, Starship, any Nerd Font setup) contains powerline separators and icon glyphs from the Unicode Private Use Area. Pasted into a Kandev prompt input, each one renders as a notdef box (square). Users read this as "Kandev deleted my character and replaced it with a square".

## Requirements

### REQ-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001: Render Nerd Font glyphs pasted from a styled terminal

**Intent:** Text copied from a styled terminal prompt (Oh My Posh, Starship, any Nerd Font setup) contains powerline separators and icon glyphs from the Unicode Private Use Area. Pasted into a Kandev prompt input, each one renders as a notdef box (square). Users read this as "Kandev deleted my character and replaced it with a square".

#### Acceptance criteria

- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.1:** Prompt inputs and message surfaces SHALL render Private Use Area codepoints as their intended glyphs.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.2:** Resolution SHALL prefer a Nerd Font the user already has installed, and fall back to a subset shipped by Kandev when they have none.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.3:** A local-only face SHALL list `local()` sources for the full Nerd Fonts catalogue (66 patched families). A separate bundled-only face SHALL contain the `url()` source. Every application font stack SHALL put the local face before the bundled face, so an installed font wins and no download occurs.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.4:** Every `local()` entry SHALL name a **full font name** (family plus style, for example `MesloLGS Nerd Font Regular`) or a PostScript name. `local()` does not match family names: `local("MesloLGS")` fails silently where `local("MesloLGS Nerd Font Regular")` resolves. This is the inverse of the `font-family` property and the failure is invisible, since an unmatched source simply falls through.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.5:** Ordering SHALL express intent rather than alphabetical accident, because glyph shapes differ between families and the closest match to the user's terminal is preferred: icons-only `Symbols`, then the Meslo variants that Oh My Posh and Powerlevel10k recommend, then common programming faces, then the remainder.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.6:** Only Nerd Font patched faces SHALL be listed. Within one `src` list, a resolved local resource does not retry the later URL for a missing glyph. Separate font families let the browser continue from the selected local resource to the bundled face for each missing character. Unpatched `Cascadia Code` was listed as a Windows safety net and removed because it covers 0 of the powerline, seti, devicon and octicon codepoints.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.7:** Distributions of the same family may use different names, so covering the Nerd Fonts release alone is not sufficient. Powerlevel10k ships its own MesloLGS build, the most common way a styled-prompt user acquires a patched font, declaring full name `MesloLGS NF Regular` and PostScript `MesloLGS-NF-Regular`, where the v3.5.0 release file for the same family declares `MesloLGS Nerd Font Regular` / `MesloLGSNF-Regular`. Both are listed.
- **AC-UI-PROMPT-PASTE-NERD-FONT-GLYPHS-001.8:** Patched family names are not derivable from the original typeface (`Source Code Pro` becomes `SauceCodePro`, `Cascadia Code` becomes `CaskaydiaCove`, `IBM Plex Mono` becomes `BlexMono`), so the catalogue is enumerated rather than inferred.

## Migrated source detail

## Why
Text copied from a styled terminal prompt (Oh My Posh, Starship, any Nerd Font
setup) contains powerline separators and icon glyphs from the Unicode Private
Use Area. Pasted into a Kandev prompt input, each one renders as a notdef box
(square). Users read this as "Kandev deleted my character and replaced it with
a square".

Nothing is deleted, and nothing is corrupted. The paste, the editor, the
WebSocket, the database and the agent all handle the text correctly: `U+E0B0`
is transmitted as its correct UTF-8 encoding `EE 82 B0`. The failure is purely
at render time, because no font in the UI stack has a glyph for those
codepoints.

PUA codepoints cannot be resolved by platform font-matching the way CJK or
emoji are. Those carry a script and language association, so the OS can
nominate a covering font. The Private Use Area deliberately carries none:
`U+E0B0` means whatever the installed font decides. The OS has nothing to match
on, returns no font, and the browser draws notdef. A covering font must be
named explicitly, which is the same reason a user must configure their terminal
font to see powerline glyphs; installing the font is not sufficient.

## What
- Prompt inputs and message surfaces SHALL render Private Use Area codepoints
  as their intended glyphs.
- Resolution SHALL prefer a Nerd Font the user already has installed, and fall
  back to a subset shipped by Kandev when they have none.

### Font resolution order
- A local-only face SHALL list `local()` sources for the full Nerd Fonts
  catalogue (66 patched families). A separate bundled-only face SHALL contain
  the `url()` source. Every application font stack SHALL put the local face
  before the bundled face, so an installed font wins and no download occurs.
- Every `local()` entry SHALL name a **full font name** (family plus style, for
  example `MesloLGS Nerd Font Regular`) or a PostScript name. `local()` does
  not match family names: `local("MesloLGS")` fails silently where
  `local("MesloLGS Nerd Font Regular")` resolves. This is the inverse of the
  `font-family` property and the failure is invisible, since an unmatched
  source simply falls through.
- Ordering SHALL express intent rather than alphabetical accident, because
  glyph shapes differ between families and the closest match to the user's
  terminal is preferred: icons-only `Symbols`, then the Meslo variants that Oh
  My Posh and Powerlevel10k recommend, then common programming faces, then the
  remainder.
- Only Nerd Font patched faces SHALL be listed. Within one `src` list, a
  resolved local resource does not retry the later URL for a missing glyph.
  Separate font families let the browser continue from the selected local
  resource to the bundled face for each missing character. Unpatched
  `Cascadia Code` was listed as a Windows safety net and removed because it
  covers 0 of the powerline, seti, devicon and octicon codepoints.
- Distributions of the same family may use different names, so covering the
  Nerd Fonts release alone is not sufficient. Powerlevel10k ships its own
  MesloLGS build, the most common way a styled-prompt user acquires a patched
  font, declaring full name `MesloLGS NF Regular` and PostScript
  `MesloLGS-NF-Regular`, where the v3.5.0 release file for the same family
  declares `MesloLGS Nerd Font Regular` / `MesloLGSNF-Regular`. Both are
  listed.
- Patched family names are not derivable from the original typeface
  (`Source Code Pro` becomes `SauceCodePro`, `Cascadia Code` becomes
  `CaskaydiaCove`, `IBM Plex Mono` becomes `BlexMono`), so the catalogue is
  enumerated rather than inferred.

### Bundled subset
- Kandev SHALL ship a subset of `Symbols Nerd Font` (the icons-only Nerd Fonts
  build, MIT licensed) so users with no Nerd Font installed still see glyphs.
- The subset SHALL cover powerline separators (`U+E0A0-E0D4`), octicons
  (`U+F400-F533`), seti/custom file and folder icons (`U+E5FA-E6B7`), and
  devicons (`U+E700-E8EF`): the four sets an Oh My Posh prompt actually draws.
- The bundled face SHALL declare the subset's actual character map, including
  gaps in the powerline range. An unsupported PUA codepoint SHALL NOT start a
  download that cannot render it. The local face SHALL keep all three PUA
  ranges so an installed font can render glyphs outside the subset.
- The subset SHALL be served from the application's own origin, never a
  third-party CDN, which would leak that a user pasted terminal output and add
  a runtime dependency on someone else's uptime.
- The subset filename SHALL contain the first eight characters of its SHA-256
  hash. Kandev serves static assets with a one-year immutable cache policy, so
  a changed binary SHALL use a changed URL.
- Because the bundled face declares its exact `unicode-range`, the browser
  fetches it only when a supported PUA codepoint needs that face. A user who
  never renders supported PUA text downloads nothing.

### Stack coverage
- The local and bundled glyph families SHALL be present in that order in every
  font stack the stylesheet declares, not only the `--font-sans` and
  `--font-mono` variables.
  `.markdown-body` (rendered markdown across chat, PR and changelog) and
  `.chat-message-list` hardcode their own families rather than reading the
  variables, so a fix applied only to the variables leaves sent messages and
  agent output rendering notdef while the composer renders glyphs.

### Sizing
- Both faces SHALL carry a `size-adjust` descriptor. Powerline separators are
  drawn to fill a full terminal cell, ascender to descender, measuring ~1.99x
  the cap height of the UI typeface, and read as oversized blocks beside
  proportional text.

### Text sent to the agent is not modified
- The bytes delivered to the backend and the agent SHALL be byte-for-byte
  identical to before this feature. No sanitization, stripping, substitution,
  or normalization is applied to pasted text on any path.
- This is a hard requirement. Private Use codepoints may be meaningful to the
  agent: a user pasting a prompt theme, a powerline configuration, or terminal
  output may be asking about those exact characters, and removing them would
  delete the subject of the question.
- The implementation satisfies this by construction rather than by discipline:
  the feature is CSS only, and CSS cannot alter text content.

## Failure modes
- **No Nerd Font installed, glyph inside the bundled subset.** The bundled
  woff2 is fetched once and the glyph renders. Roughly 236 KB, cached, and
  never fetched by users who paste no PUA text.
- **No Nerd Font installed, glyph outside the subset.** Notdef box, as today.
  Affects the plane-15 Material Design range (`U+F0000-FFFFD`) and the Font
  Awesome block, both excluded to keep the download small. The browser does
  not fetch the subset for these unsupported codepoints. Widening requires
  regenerating the subset from the pinned source archive, changing the hashed
  filename, and updating its recorded hash.
- **An installed Nerd Font lacks one requested glyph.** The browser continues
  to the separate bundled family for that character. One combined `src` list
  would stop at the selected local resource instead.
- **A Nerd Font installed that is absent from the catalogue.** The local
  sources miss, and resolution falls through to the bundled subset, so the
  common glyphs still render.
- **A font in the stack claims a PUA codepoint unexpectedly.** `unicode-range`
  scoping confines both faces to the PUA, so no ordinary text can change
  appearance.

## Scenarios
- **GIVEN** a user with a Nerd Font installed, **WHEN** they paste an Oh My
  Posh prompt containing `U+E0B0`, **THEN** the separator renders using their
  own font, matching the terminal they copied from, and no font is downloaded.
- **GIVEN** a user with no Nerd Font installed, **WHEN** they paste the same
  text, **THEN** the bundled subset is fetched and the separator, git branch,
  folder and devicon glyphs render.
- **GIVEN** a user who never pastes Private Use text, **WHEN** they use the
  app, **THEN** the bundled subset is never requested.
- **GIVEN** any paste, **WHEN** the message is sent, **THEN** the backend and
  the agent receive exactly the same bytes as before this feature, including
  the Private Use codepoints.
- **GIVEN** ordinary text (accented Latin, CJK, emoji, box drawing), **WHEN**
  it is rendered, **THEN** it uses the existing UI typeface with unchanged
  metrics.
- **GIVEN** a maintainer edits the rule, **WHEN** a `local()` entry is reduced
  to a family name, the bundled family is moved ahead of the local family, the
  `url()` loses its content hash, the bundled range exceeds its character map,
  or `size-adjust` is dropped, **THEN** the guard test fails.

## Out of scope
- Shipping a complete Nerd Font. Only the icons-only subset is bundled, and
  only for the four icon sets a terminal prompt uses.
- Modifying, stripping, or normalizing pasted text on any path. Explicitly
  rejected: the agent must receive the characters.
- Rendering terminal background colours. Those are painted by the terminal and
  are not present in the clipboard as text; reproducing them would require ANSI
  parsing plus colour marks in the editor schema, a separate feature.
- Making the glyph font user-configurable. The terminal already exposes
  `terminalFontFamily`; a separate preference for the composer is not justified
  until someone asks for it.
