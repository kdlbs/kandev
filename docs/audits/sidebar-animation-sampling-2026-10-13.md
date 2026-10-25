# Sidebar animation sampling

The three-process hosted trial (E2E run 38425537974, commit 3314a962)
failed the sidebar geometry case on all four attempts. The final panel width
differed from the layout width by 11.797, 42.406, 7.375 and 1.766 pixels.
The sampler stopped after 400 milliseconds even when the browser was still
animating. Its final frame was therefore not necessarily the settled frame.

Retain this case: it detects layout changes in the right pane while the left
sidebar animates. Neither shortcut behavior nor static collapsed geometry
provides that coverage. The production owner remains the sidebar component;
this change only repairs the test's observation boundary.

The sampler now waits for the requested collapsed state and for the sidebar's
own animations to finish, with a bounded five-second failure deadline. It does
not wait for descendant animations. All existing assertions remain: the CSS
width transition is 290–310 milliseconds, intermediate frames exist, the
layout shell changes once, the visual width changes by at least 199 pixels,
the visual animation outlasts the layout change, right-pane geometry stays
within one pixel, and final sidebar/layout widths agree within one pixel.
The test timeout and retry policy are unchanged.

## Verification

A temporary Chromium DevTools probe set animation playback to 0.5 without
changing the CSS duration. The original sampler failed the same final-width
assertion. The repaired sampler passed with every original assertion intact.
The controller restored the original source bytes after each probe and saved
hash receipts. The playback override is not part of the committed test.

The complete unmodified three-case file passed with retries disabled after
the repair, and again after extracting the pending-state predicate to satisfy
the complexity limit. ESLint passes for this file and the queue consolidation.
Logs, reports and restoration receipts are under ignored
`apps/backend/.build/ci-half/e2e-focus/sidebar-completion-*`.

This proves the fixed sampling window can cause the observed failure and that
the repaired observer handles delayed playback. A passing hosted run is still
required; it does not establish that all concurrent-run failures share this
cause or that the full-suite runtime target has been met.
