# Native Markdown input checks

Open the isolated evaluation task supplied in `TESTING-PR3113.md`.
Allow 12 minutes per desktop browser and 8 minutes per phone.
Use disposable fixture files. Record the browser, OS, input method, and result.

Automated Linux Chromium, Firefox, and WebKit checks do not establish native
macOS Safari, Zen, iOS, Android keyboard, or screen-reader acceptance. The
checks below remain pending until someone performs them on those systems.

## Desktop browser checks

1. In current Zen/Firefox on macOS, open `harbor-docs/README.md` from Files.
   Select Edit. Change a rendered paragraph and a rendered table cell. Switch
   through Preview and Source. Save, reload, and check that both changes remain.
   Repeat in current Safari. Edit must remain available in both browsers.
2. Select text backward with the mouse. Copy it, move the caret, and paste.
   Cut another word. Undo and redo. Each command must apply once. A selection
   or copy action alone must not enable Save. Repeat with keyboard selection.
3. Enable a Japanese or Chinese IME. Select an existing word and compose its
   replacement. Check the candidate window near the active text. Start another
   composition at another location. Switch to Preview during composition.
   Confirm the final accepted text in Source. Cancel another composition.
   No text may disappear, duplicate, or move to the wrong range.
4. Select rendered text and open its comment action. Type a comment, submit,
   and return to the editor. Open Find. Return to the editor again. Text entry
   must reach the focused control. Comment creation must not change file bytes.
5. In a long document, select a word near paragraph 40. Switch through Preview,
   Source, and Edit. The region must stay visible. Edit a paragraph in
   `preservation.md`. Undo and redo, then save. Unrelated mixed line endings,
   frontmatter, code, HTML, and whitespace must remain unchanged.

## Native phone checks

Use real Android Chrome and Firefox, then real iOS Safari. Responsive emulation
is additional layout evidence and does not replace these checks.

1. Open Files and the README. Select Edit. Tap a paragraph below the fold.
   The virtual keyboard must open near the active text without hiding Save.
2. Use native selection handles, composition, autocorrect, and replacement.
   Repeat in a table cell. Scroll while the keyboard is open. The caret and
   candidate UI must stay near the selected text.
3. Insert a table row and column. Resize a column with a real touch drag.
   Wide tables must scroll locally. The page must not scroll horizontally.
   Automated WebKit touch-pointer dispatch does not prove this OS gesture.
4. Rotate with an unsaved draft. Switch modes, save, reload, and reopen the
   file. The latest accepted text must survive. Open the actions menu and
   confirm that mode choices and Reload remain reachable at narrow widths.

## Screen-reader checks

Use NVDA with Firefox on Windows and VoiceOver with Safari on macOS/iOS.
Allow 5 minutes per combination.

1. Navigate to Edit using the keyboard or screen-reader controls. Find the
   native text control named "Content (markdown supported)" in English, or its
   localized equivalent. Read the current text and selection.
2. Select backward, replace a word, and read the result. The same text must
   not be announced twice through a second decorative editor surface.
3. Navigate to comments, table actions, the mode control, and Save. Return to
   the editor. Focus must return to the selected text without a keyboard trap.

## Record results

For each check, record the browser version, OS version, viewport/device,
keyboard or assistive technology, action, expected result, and actual result.
Attach screenshots for geometry failures. Keep missing device access marked
pending. Do not report it as a passing automated check.
