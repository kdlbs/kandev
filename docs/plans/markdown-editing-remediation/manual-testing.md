# Markdown editing evaluation

> Browser correction, 2026-10-21: this checklist records the earlier limited
> browser build. The user requires rendered Edit in Firefox/Zen and Safari.
> Treat unavailable Edit as a failure against that requirement. See the
> [browser parity correction](../markdown-browser-parity/plan.md). Earlier
> Chromium checks do not establish cross-browser acceptance.

Allow about 12 minutes. Use a current Chrome or Edge browser for hybrid Edit.
Use the isolated evaluation URL supplied with this guide.

The workspace contains two fictional repositories, `harbor-docs` and
`harbor-handbook`. Both contain a different `README.md`. The task
**Evaluate Markdown editing** opens both repositories. Its agent is simulated.
You can edit and save the fixture files without affecting the source checkout.

1. **Check the three modes (2 minutes).** Open the task, select Files, and open
   `harbor-docs/README.md`. A new file starts in Preview. Select Edit, change
   one paragraph, and switch through Preview and Source. Each mode must show
   the same unsaved change. Select Save, reload the browser, and reopen the
   file. The saved change must remain. Open `components.mdx`; it must offer
   Preview and Source only.
2. **Check reading position (1 minute).** In Edit, find **Long paragraph 40**.
   Select a word. Switch to Preview, then Source. The same region must remain
   visible. Type a replacement in Source and return to Edit. Only the selected
   text must change. Save the file.
3. **Check repository comments (2 minutes).** Open `harbor-handbook/README.md`
   in Edit. Select text and use the comment control to add a unique comment.
   Open `harbor-docs/README.md`; that comment must not appear there. Return to
   the handbook README; the comment must reappear. Adding a comment must not
   mark the file dirty.
4. **Check phone and tablet retention (2 minutes).** Set browser responsive
   width to 767px. Open Files, choose the main README, select Source, and type
   a marker. Change the width to 768px. Open the same file in the tablet Files
   surface. The unsaved marker must remain. Save there, return to 767px, and
   reopen the file. The marker must remain and Save must be disabled.
5. **Check narrow controls and outside writes (2 minutes).** Use width 320px
   and a viewport height near 400px. Make a local change. Run the outside-write
   command supplied with the evaluation instance. The remote-update indicator
   must appear while your local draft remains intact. Save must remain visible.
   Open the actions menu and select Reload. This explicitly replaces your local
   draft with the outside version. Repeat in Portuguese if useful. The mode
   menu and Reload must have usable touch targets. Escape must return focus to
   the menu trigger.
6. **Check leaving a dirty file (1 minute).** Type another marker. Use Back or
   switch to **Check navigation safety**. Cancel the discard decision. You must
   remain with the unsaved marker. Repeat and choose discard. Reopening the
   file must show its saved content.
7. **Check tables and source preservation (2 minutes).** In Edit, insert a table
   row and column. Resize a column with pointer or touch. Insertions must alter
   Markdown source. Resizing must not add width metadata to the file. Edit a
   paragraph in `preservation.md`, undo, redo, and save. Frontmatter, fenced
   code, raw HTML, footnotes, and unrelated Markdown must remain intact.

If Edit is unavailable, Preview and Source must still work. The browser must
explain the unavailable state. An unsupported browser does not establish the
hybrid Edit checks. Native Safari and Firefox input remain outside the current
verified browser coverage.

The retry behavior is covered by automated tests that inject an engine failure.
For manual evaluation, a failed Edit must preserve the draft in Source. Selecting
Edit again must create a working editor when the browser supports it.

Record the file, browser, viewport, action, expected result, and actual result
for any issue. Include a screenshot when layout or selection is involved.

The evaluation instance has its own shutdown command. Use that exact command.
Do not stop the main instance on port 9998.
