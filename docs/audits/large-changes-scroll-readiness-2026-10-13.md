# Large Changes final-row scrolling

Run 38448127809 fails the mobile 50,000-file list case at 767px. The final row
ends at y=771 while its scroll owner's visible bottom is y=757. The owner can
still scroll another 26px: scrollHeight 2,400,092 minus clientHeight 635 minus
scrollTop 2,399,431. The screenshot confirms the clipped row. This is not a
missing last file or proof that the virtualized list cannot reach it.

The helper initially jumps to the estimated end, then manually corrects the
row position for at most four animation frames. Presentation changes refresh
virtualizer measurements and restore an anchor asynchronously. A fixed number
of corrections is not an element-stability condition. Replace that loop with
Playwright's `scrollIntoViewIfNeeded()` after the last row mounts. The built-in
action waits for element stability and scrolls the actual element into view.

Keep the final geometry assertion, row-count bound, all 50,000 files, both
393px and 767px touch viewports, long-name wrapping, touch controls, keyboard
navigation and repeated start/end scrolling. No product code, fixture scale,
retry count or assertion timeout changes. This removes a custom scroll
fallback rather than a coverage scenario.

The unchanged complete mobile file passes six cases across two local
iterations with zero retries; that does not negate the hosted failure.
The complete desktop and mobile files pass all six cases with the replacement
in the pinned CI image, one worker and zero retries. Changed-helper lint and
format checks pass. Fresh full hosted validation remains pending. Evidence remains under ignored `e2e-focus/mobile-large-*`
and `e2e-focus/large-scroll-native*`.
