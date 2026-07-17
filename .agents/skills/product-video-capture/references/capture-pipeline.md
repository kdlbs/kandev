# Capture Pipeline

## Why X11 Capture

Playwright `recordVideo.size` can report a large container while preserving only CSS-pixel content in the top-left and padding the rest. Container dimensions alone therefore do not prove native DPR detail.

Preferred Linux route:

1. Start a worker-scoped Kandev E2E backend.
2. Launch headed Chrome for Testing in app mode under Xvfb.
3. Force device scale factor and set CSS viewport/window dimensions deliberately.
4. Connect Playwright over CDP for real mouse/touch input.
5. Record the X display with FFmpeg `x11grab` at physical dimensions.

Current established profiles are:

| Form | CSS viewport | DPR | Raw physical frame |
| --- | ---: | ---: | ---: |
| Desktop | 1920x1200 | 2 | 3840x2400 |
| Mobile | 430x932 | 3 | 1290x2796 |

Verify current landing encoder constants before capture. If dimensions change, update encoder tests first.

## Browser Shape

Launch Chrome with an isolated user-data dir and flags equivalent to:

```text
--disable-infobars
--hide-scrollbars
--force-device-scale-factor=<dpr>
--window-position=0,0
--window-size=<css-width>,<css-height>
--app=<isolated-web-url>
--remote-debugging-port=<free-cdp-port>
```

Use a fresh Xvfb display sized to the physical frame. Confirm the decoded first frame fills that frame; do not infer this from `ffprobe` width/height alone.

## Recorder Shape

Record a visually lossless working master:

```text
ffmpeg -f x11grab -draw_mouse 0 -framerate 25 \
  -video_size <physical-width>x<physical-height> -i <display> \
  -an -c:v libx264 -preset ultrafast -crf 10 -pix_fmt yuv420p <raw.mp4>
```

Start FFmpeg immediately before the opening beat and wait until it reports a real frame. Stop it cleanly with `q`; wait for a zero exit code before closing Chrome or taking a poster.

## Pointer And Touch

Use one capture-only overlay:

- desktop: high-contrast pointer with restrained click pulse;
- mobile: small touch ring and pulse;
- OS pointer disabled through `-draw_mouse 0`;
- pointer overlay hidden for poster capture.

Keep the complete rendered glyph inside the raw viewport. Preserve the hotspot on the real target and switch to an edge-safe glyph orientation near the right or bottom edge instead of clipping the overlay. Record both the hotspot and glyph bounds so post-camera validation can use the visible footprint.

Move real input and overlay together. Use 10-12 manually timed samples over roughly 300ms with cubic easing. Record for every arrival:

```json
{
  "action": "cursor-arrive",
  "at_ms": 2400,
  "motion_ms": 300,
  "motion_samples": 12,
  "from": { "x": 320, "y": 180 },
  "to": { "x": 1080, "y": 720 },
  "target_bounds": { "x": 1010, "y": 680, "width": 220, "height": 64 },
  "label": "Open changed file"
}
```

Bounds use CSS pixels. Keep targets inside viewport and store the raw-story start time separately from event story time. Persist enough waypoints to reconstruct the complete intentional journey: the previous settled position, movement start, intermediate samples or easing contract, arrival, and the next movement start. Click-only metadata cannot prove cursor containment.

Before camera work, normalize pointer coordinates against the exact camera source. For full-frame capture, divide CSS coordinates by the CSS viewport. For a physical-pixel ROI, first multiply CSS coordinates by DPR, subtract the ROI origin, then divide by ROI dimensions. Reject a static ROI when any visible pointer waypoint falls outside it; use a dynamic full-frame camera instead.

## Choreography

- Rehearse before recording; do not discover selectors during a take.
- Opening frame should establish context for 300-600ms.
- Each action should visibly land before the next begins.
- Remove dead waits by controlling seed timing, not editing time.
- Show one meaningful state change every 1-2 seconds.
- End on a readable result, then hold a settled frame long enough to loop cleanly.
- Keep capture duration near 7-11 seconds unless the requested format explicitly differs.

## Native Mobile

Use a mobile Playwright project with `isMobile` and touch enabled, a separate script, and native mobile navigation. Do not replay desktop coordinates or selectors. Assert complete stage labels, sheets, bottom navigation, and action buttons before recording.

When desktop uses a split diff but mobile exposes `MobileDiffSheet`, write different choreography around the same user outcome.

## Raw Rejection Conditions

Reject and recapture if any sampled or playback frame shows:

- CSS-size content padded inside a larger gray/black canvas;
- browser chrome, host desktop, notification, or URL tooltip;
- double cursor or OS cursor;
- loader/blank/error state held as a story beat;
- generic fixture/mock text, slash directives, local paths, localhost status, or host identity;
- clipped menu, task title, stage label, diff, composer, or primary action;
- product UI bug hidden through capture-only CSS;
- unexplained time jump, cursor teleport, or state transition.

## Reproducibility Bundle

Keep outside production asset directory until review:

```text
capture-root/
|-- raw/<slug>.mp4
|-- posters/<slug>.png
|-- metadata/<slug>.json
|-- frames/<slug>/{10,25,50,75,90}.png
|-- mosaics/<slug>.png
|-- source/<capture-spec-and-helper>
|-- NOTES.md
`-- SHA256SUMS
```

Metadata should record seed narrative, form factor, CSS/DPR/raw dimensions, ports, story start/duration, semantic events, capture command, hashes, and visual audit results.
