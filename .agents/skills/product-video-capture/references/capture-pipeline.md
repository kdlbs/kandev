# Capture Pipeline

## Source Gate And Isolation

Record only from a clean worktree at the fetched `origin/main` commit. A product capture is evidence of current Kandev; another branch, tag, or historical revision is not an eligible substitute. Before build or rehearsal:

1. Fetch `origin/main`, create a clean detached worktree at that exact commit, and record the resolved SHA.
2. Build inside that worktree. Reject stale UI, a reused build from another checkout, or a local branch that merely claims to be current.
3. Start a worker-scoped Kandev E2E backend with a unique temp home, database, repository root, and port range. Never attach to a developer instance, developer database, or production port.
4. Allocate a unique X display, CDP port, and browser profile for each capture worker.
5. Put raw, metadata, configs, proofs, QA, and delivery candidates under a unique `CAPTURE_ROOT` outside every production asset directory.

Use deterministic demo seeds from `product-demo-seeding`. Rehearse without recording, then reset to a fresh per-take database or deterministically remove every task/session created by rehearsal. A final take must not show duplicate fixtures or accumulated provider-created tasks.

Set the requested theme through a normal Kandev user setting or browser `prefers-color-scheme` preference. Never patch the DOM or inject CSS to imitate a theme. Save an exact-profile rehearsal frame that proves the theme before recording.

## Why X11 Capture

Playwright `recordVideo.size` can report a large container while preserving only CSS-pixel content in the top-left and padding the rest. Container dimensions alone do not prove native DPR detail.

Preferred Linux route:

1. Launch headed Chrome for Testing in app mode under Xvfb.
2. Force device scale factor and set CSS viewport/window dimensions deliberately.
3. Connect Playwright over CDP for real mouse/touch input.
4. Record the X display with FFmpeg `x11grab` at physical dimensions.

Authoritative profiles:

| Form | CSS viewport | DPR | Source master | Delivery |
| --- | ---: | ---: | ---: | ---: |
| Desktop | 1920x1200 | 2 | 3840x2400 at 25 fps | 1920x1200 at 25 fps |
| Mobile | 430x932 | 3 | 1290x2796 at 25 fps | 1290x2796 at 25 fps |

If a requested format differs, update the landing encoder tests first instead of silently substituting an old profile. Never crop desktop footage into a mobile delivery.

## Browser And Recorder Shape

Launch Chrome with an isolated user-data directory and flags equivalent to:

```text
--disable-infobars
--test-type
--hide-scrollbars
--force-device-scale-factor=<dpr>
--window-position=0,0
--window-size=<css-width>,<css-height>
--app=<isolated-web-url>
--remote-debugging-port=<free-cdp-port>
```

Use a fresh Xvfb display sized to the physical frame. Confirm decoded frames fill all four edges; do not infer this from `ffprobe` width and height alone.
Decode every raw frame and inspect the browser-managed perimeter in frame-indexed contact sheets for Chrome's automation banner; first, middle, and final samples cannot exclude a transient artifact. `--disable-infobars` alone is not sufficient on every Chrome for Testing build; reject the take if any browser-managed banner remains.

Measure realtime recorder/encoder capacity at the exact source profile before RECORD. The probe must sustain capture while the browser story runs, and its FFmpeg log must report zero duplicated and dropped frames. Hardware acceleration is acceptable when it produces a visually lossless, probeable master; otherwise choose a software preset that has demonstrated headroom on the capture host. Never accept nominal 25 fps metadata when the recorder duplicated frames to catch up.

Record a visually lossless working master as one continuous take at 25 fps with no internal cuts, speed ramps, or audio. The following software example is valid only after that measured-capacity gate:

```text
ffmpeg -f x11grab -draw_mouse 0 -framerate 25 \
  -video_size <physical-width>x<physical-height> -i <display> \
  -an -c:v libx264 -preset ultrafast -crf 10 -pix_fmt yuv420p <raw.mp4>
```

Start FFmpeg immediately before the opening beat and wait until it reports a real frame. Stop it cleanly with `q`; wait for a zero exit code before closing Chrome. A single beginning/end trim may remove recorder startup, but there is no concat or internal time edit.

Archive the complete FFmpeg log. Reject the take unless it proves zero duplicated and dropped frames, output duration aligns with recorded story start/end marks, frame timestamps advance at 40ms cadence, and the encoder sustained realtime capacity for the full take.

## Pointer And Touch Metadata

Use one capture-only overlay:

- desktop: high-contrast pointer with restrained click pulse;
- mobile: small touch ring and pulse;
- OS pointer disabled through `-draw_mouse 0`;
- overlay hidden only after recording for a clean poster.

Move real input and the overlay together through one eased, time-sampled trajectory. Keep the DOM overlay and real browser pointer in lockstep. At every sample, make the trusted mousemove event on desktop, or the corresponding trusted pointer/touch event on mobile, the sole source of truth. Send real Playwright/CDP input and let that trusted event update both the DOM cursor from its event coordinates and the semantic metadata ledger. Never animate it independently with `requestAnimationFrame` and then move the browser pointer afterward: that later input will override the overlay and create a visible teleport. Keep the complete rendered glyph inside the raw viewport. Preserve the hotspot on the real target, using an edge-safe glyph orientation near the right or bottom edge instead of clipping it.

Direct setup input can leave a travel helper's remembered origin stale even when the browser is correct. Re-sync the current pointer from a trusted setup click or movement before RECORD and before every semantic story that follows direct Playwright setup input.

Use adaptive sample waits calculated from the measured elapsed time of the preceding trusted input event. A fixed sleep is insufficient under recording load: it can turn a smooth rehearsal into stepped 45-90ms movement during X11 capture. Target roughly 32ms between trusted movement events, retain a small non-zero wait rather than issuing catch-up bursts, and validate the RECORD stream itself. Reject a take when meaningful travel has p95 above 56ms or a maximum interval above 64ms, even if camera metadata interpolates smoothly.

Exercise the travel helper in a focused contract test before RECORD. The test must prove that intermediate overlay and browser-pointer positions stay aligned, the final real-input update causes no residual overlay displacement, and click/hover begins only after both sources reach the same destination. Inspect the resulting motion at normal playback speed; metadata interpolation alone cannot prove that recorded pixels are smooth.

Record dense semantic pointer/touch metadata for every intentional movement: previous settled point, motion start, 10-12 timed intermediate samples or an equally dense tested easing contract, arrival, next motion start, and visibility interval. Click-only timestamps cannot prove containment. Each event records target bounds, target glyph bounds, pointer/touch glyph bounds, hotspot, label, and coordinates in CSS pixels:

Target glyph bounds measure the rendered visual content inside the semantic target or control; pointer glyph bounds measure the separate rendered cursor/touch overlay. Never reuse pointer or cursor geometry as target glyph bounds. The visibility interval starts at motion start, not arrival, and ends only at the next movement or final settled hold.

```json
{
  "action": "cursor-arrive",
  "at_ms": 2400,
  "motion_ms": 300,
  "motion_samples": 12,
  "from": { "x": 320, "y": 180 },
  "to": { "x": 1080, "y": 720 },
  "target_bounds": { "x": 1010, "y": 680, "width": 220, "height": 64 },
  "target_glyph_bounds": { "x": 1028, "y": 698, "width": 18, "height": 18 },
  "pointer_glyph_bounds": { "x": 1072, "y": 712, "width": 28, "height": 34 },
  "visibility_interval": { "start_ms": 2100, "end_ms": 3600 },
  "samples": [
    { "at_ms": 2100, "x": 320, "y": 180 },
    { "at_ms": 2250, "x": 700, "y": 450 },
    { "at_ms": 2400, "x": 1080, "y": 720 }
  ],
  "label": "Open changed file"
}
```

Normalize pointer coordinates against the exact camera source. For full-frame capture, divide CSS coordinates by the CSS viewport. For a physical-pixel ROI, first multiply CSS coordinates by DPR, subtract the ROI origin, then divide by ROI dimensions. Reject a static ROI when any visible pointer waypoint falls outside it; use the reversible full-frame camera.

## Choreography

- Rehearse selectors and the complete desktop/mobile route before recording.
- Establish context for 300-600ms.
- Land each pointer or touch target before activating it.
- Remove dead waits through deterministic seed timing, not editing time.
- End on a readable result and hold a settled frame long enough for a calm loop.
- End after the requested product beat. Do not append unrelated route loading, environment setup, terminal cleanup, or session readiness merely because the fixture can observe it; validate those separately from the recorded story.
- Treat 7-11 seconds as a target, never a reason to compromise an honest interaction, settled loop, or actual-size readability.

Use a mobile Playwright context with `isMobile` and touch enabled, a separate script, and native mobile navigation. Do not replay desktop coordinates or selectors. Assert complete sheets, menus, bottom navigation, labels, and action buttons before recording.

## Raw Rejection Conditions

Reject and recapture if any sampled or playback frame shows:

- CSS-size content padded inside a larger gray or black canvas;
- browser chrome, automation banner, host desktop, notification, URL tooltip, or developer tooling;
- double cursor, OS cursor, clipped pointer/touch glyph, or cursor teleport;
- loader, blank, error state, dead wait, unexplained time jump, or hidden edit;
- fixture/mock text, slash directive, local path, localhost status, host identity, or inconsistent narrative;
- clipped menu, provider panel, dialog, sheet, task title, stage label, diff, or primary action;
- product UI hidden or altered with capture-only CSS;
- stale UI, a build not proven from current `origin/main`, or mismatched requested theme;
- duplicate or accumulated tasks left behind by rehearsal or a previous take;
- a wide shot whose product text is unreadable at actual display size.

## Reproducibility Bundle

Keep the bundle outside production assets until explicit promotion approval:

```text
capture-root/
|-- raw/<slug>.mp4
|-- posters/<slug>.png
|-- metadata/<slug>.json
|-- configs/<slug>.json
|-- delivery/<slug>.{webm,mp4,webp}
|-- proofs/<slug>/actual-size-*.png
|-- proofs/<slug>/contact-sheet.png
|-- proofs/rehearsal/<slug>-theme.png
|-- qa/{codec,browser,containment}/<slug>.*
|-- source/<capture-spec-and-helper>
|-- PROVENANCE.json
|-- NOTES.md
`-- SHA256SUMS
```

Provenance records seed identity, Kandev and landing commit SHAs, form factor, CSS/DPR/source/delivery dimensions, frame rate, ports and X display, isolated browser profile, temp database/home, story start/duration, semantic events, capture and encode commands, hashes, and visual audit results. `SHA256SUMS` covers raw, metadata, camera config, actual-size proofs, contact sheets, browser evidence, and WebM/MP4/WebP candidates.

## Teardown Evidence

After QA, stop FFmpeg, Chrome, Playwright, frontend, backend, and Xvfb in dependency order. Prove the allocated backend/web/CDP ports have no listeners and the X socket is absent. Remove only the recorded temporary spec copies, browser profiles, databases, worktrees, and temp homes; preserve the staged bundle and unrelated work. Record every resolved target and teardown check in provenance.
