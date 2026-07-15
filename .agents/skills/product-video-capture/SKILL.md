---
name: product-video-capture
description: Record, camera-process, encode, integrate, and validate polished Kandev product films, landing-page loops, screenshots, and alternate cuts from isolated demo data. Use whenever the user asks for product videos, GIF-like feature demos, smooth cursor-follow zoom, desktop/mobile captures, recaptures, new landing media, or different framing; invoke product-demo-seeding first for new visible state.
---

# Product Video Capture

Produce reusable clean masters first; derive presentation from them later.

## Prerequisites And Repo Discovery

- Require Linux Xvfb, Chrome for Testing, Playwright/CDP, FFmpeg/FFprobe, a Kandev checkout with E2E fixtures, and the landing repository.
- Resolve the Kandev root with `git rev-parse --show-toplevel`, then verify it contains `scripts/dev-isolated` and `apps/web/e2e/`.
- Resolve the landing root from `KANDEV_LANDING_REPO` when set. Otherwise search the available workspace and sibling checkouts for both `scripts/product-loop-camera.mjs` and `scripts/product-loop-encoder.mjs`; do not select a directory by name alone.
- If discovery finds zero or multiple landing candidates, ask the user to identify the checkout. Record the resolved roots as `KANDEV_REPO` and `LANDING_REPO`, and use those variables for every command and copy operation.

## Choose Deliverable

| Request | Path |
| --- | --- |
| New feature/story | Seed with `/product-demo-seeding`, then capture desktop and mobile masters |
| Different zoom/crop/pacing | Reuse approved raw master; change camera config only |
| New poster/static image | Extract a settled pointer-free frame from approved master or recapture native screenshot |
| Longer walkthrough | Keep continuous 1x source; add a tested delivery profile instead of speed ramps |
| Actual GIF required | Derive from approved video last; retain WebM/MP4 as primary web formats |

## Pipeline

1. Resolve and verify `KANDEV_REPO` and `LANDING_REPO` as described above. Do not assume task-specific absolute paths.
2. Review the seed handoff and rehearse the full native desktop/mobile story once.
3. Record one continuous, unzoomed, high-resolution master per form factor. Use true physical pixels, not a padded Playwright video canvas.
4. Record semantic action timestamps, target bounds, and pointer/touch journeys beside the raw file.
5. Stop recording before capturing the clean poster.
6. Inspect raw frames before post-production. Reject UI bugs, padding, double cursors, fixture text, dead waits, and unreadable states.
7. Build a smooth post camera from semantic events. Camera follows story focus, not every cursor twitch.
8. Encode WebM, MP4, and WebP through landing's tested camera/encoder scripts.
9. Review fixed-fraction frames and playback on desktop, native mobile, and reduced motion.
10. Copy only approved delivery assets into `public/product/loops/`; keep raw/proof files outside production unless requested.

Read [capture-pipeline.md](references/capture-pipeline.md) before recording and [camera-encoding.md](references/camera-encoding.md) before conforming media.

## Non-Negotiable Capture Properties

- Fresh isolated data; no main instance, credentials, database, or production ports.
- Separate desktop and native-mobile scripts. Never crop desktop footage into a mobile deliverable.
- Raw master is one continuous take at 1x: no body transform, camera, crop, concat, speed ramp, or internal cut.
- Disable OS cursor in X11 capture; show one intentional high-contrast DOM cursor/touch treatment.
- Use real UI input and retain semantic bounds/timestamps for camera design and audit.
- Browser chrome absent; product fills the physical frame.
- Any responsive/product defect remains visible or blocks capture. Do not hide it with capture CSS.

## Camera And Delivery

Use `<landing-repo>/scripts/product-loop-camera.mjs` and `product-loop-encoder.mjs`. Their tests define current dimensions, frame rate, maximum zoom, smoothness, loop reset, codecs, and poster quality. Change those contracts test-first when the user requests a genuinely different delivery format.

Do not remove waits with cuts or speed changes. Improve the source choreography or recapture. One trim at the beginning/end is acceptable; time skips are not.

## Acceptance Gate

Follow [qa-checklist.md](references/qa-checklist.md). Do not ship until:

- raw and delivery dimensions are proven by decoded pixels, not only container metadata;
- cadence is constant and timestamps have no gaps;
- 10/25/50/75/90% frames and full playback pass visual review;
- camera motion is smooth, reaches intended depth, and settles on centered 1x before looping;
- text, menus, diffs, pointer, and touch targets remain inside frame;
- WebM, MP4, poster, responsive source selection, lazy loading, and reduced-motion behavior pass;
- all capture processes, ports, temporary specs, and temp data are gone.

Report story, seed, form factors, raw/delivery dimensions, durations, codecs, camera profile, output sizes/hashes, visual audit, browser checks, teardown, and any unsupported or blocked surface.
