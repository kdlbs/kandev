---
name: product-video-capture
description: Record, camera-process, encode, stage, and validate polished Kandev product films, landing-page loops, screenshots, and alternate framing from isolated demo data. Use when the user asks for product videos, GIF-like feature demos, cursor-follow camera motion, desktop/mobile captures, recaptures, landing media, or different framing; always invoke product-demo-seeding first.
---

# Product Video Capture

Produce reusable clean masters first; derive presentation from them later. Preserve the raw master and camera config so every camera choice is reversible.

## Prerequisites And Repo Discovery

- Require Linux Xvfb, Chrome for Testing, Playwright/CDP, FFmpeg/FFprobe, a Kandev checkout with E2E fixtures, and the landing repository.
- Resolve the Kandev root with `git rev-parse --show-toplevel`, then verify it contains `scripts/dev-isolated` and `apps/web/e2e/`.
- Resolve the landing root from `KANDEV_LANDING_REPO` when set and verify it contains both `scripts/product-loop-camera.mjs` and `scripts/product-loop-encoder.mjs`. If the variable is unset or its path fails verification, search the available workspace and sibling checkouts for both marker files; do not select a directory by name alone.
- If discovery finds zero or multiple landing candidates, ask the user to identify the checkout. Record the resolved roots as `KANDEV_REPO` and `LANDING_REPO`, and use those variables for every command and copy operation.

## Choose Deliverable

| Request | Path |
| --- | --- |
| New feature/story | Seed with `/product-demo-seeding`, then capture desktop and mobile masters |
| Different zoom/crop/pacing | Invoke `/product-demo-seeding` to re-prove source/isolation/provenance, then reuse an approved raw master and change camera config only |
| New poster/static image | Extract a settled pointer-free frame from approved master or recapture native screenshot |
| Longer walkthrough | Keep continuous 1x source; add a tested delivery profile instead of speed ramps |
| Actual GIF required | Derive from approved video last; retain WebM/MP4 as primary web formats |

## Pipeline

1. Always invoke `/product-demo-seeding` before this skill. For alternate framing, it validates provenance and isolation instead of manufacturing new visible state. An approved raw is reusable only when its source SHA still equals freshly fetched `origin/main`; otherwise recapture. Then resolve and verify `KANDEV_REPO` and `LANDING_REPO` as described above. Do not assume task-specific absolute paths.
2. Create a unique writable `CAPTURE_ROOT`, for example with `mktemp -d "${TMPDIR:-/tmp}/kandev-product-capture.XXXXXX"`. Use it for every raw, proof, config, and staged delivery path; do not write candidates into production assets.
3. Review the seed handoff and rehearse the full native desktop/mobile story once. Reject stale UI, fixture accumulation, duplicate tasks, selector drift, or a theme that differs from the requested product surface before starting the recorder.
4. Reset to a fresh per-take database or deterministically clean all rehearsal-created state. Record one continuous take as an unzoomed high-resolution master per form factor. Use true physical pixels, not a padded Playwright video canvas.
5. Record semantic action timestamps, target bounds, target glyph bounds, and dense pointer/touch metadata beside the raw file. Include each intentional movement's start, intermediate samples, arrival, pointer glyph bounds, and visibility interval.
6. Stop recording before capturing the clean poster.
7. Inspect raw frames before post-production. Reject UI bugs, padding, double cursors, fixture text, dead waits, and unreadable states.
8. Build a smooth, center-biased post camera from semantic events. Ignore micro-jitter, but keep every intentional pointer/touch journey inside the tested safe frame. For long travel, widen, pan with the active pointer, then tighten after arrival; complete menus, provider panels, sheets, and dialogs have framing priority.
9. Encode WebM, MP4, and WebP through landing's tested camera/encoder scripts.
10. Review fixed-fraction frames and playback on desktop, native mobile, and reduced motion. For landing films, also review at the actual theater width; full-resolution frames alone do not prove readable marketing media.
11. Keep all candidates staged outside production. Promotion is a separate, explicit action after approval; a capture request alone does not authorize overwriting landing assets.

Read [capture-pipeline.md](references/capture-pipeline.md) before recording and [camera-encoding.md](references/camera-encoding.md) before conforming media.

## Non-Negotiable Capture Properties

- Fresh isolated data; no main instance, credentials, database, or production ports.
- Separate desktop and native-mobile scripts. Never crop desktop footage into a mobile deliverable.
- Exact profiles: desktop source `3840x2400` at 25 fps delivers `1920x1200` at 25 fps; native mobile source and delivery are `1290x2796` at 25 fps.
- Raw master is one continuous take at 1x with no internal cuts, speed ramps, or audio. It also has no body transform, camera, crop, or concat.
- Disable OS cursor in X11 capture; show one intentional high-contrast DOM cursor/touch treatment.
- Use real UI input and retain dense semantic cursor/touch samples, target bounds, target glyph bounds, rendered glyph bounds, and visibility intervals for camera design and audit.
- Browser chrome absent; product fills the physical frame.
- Any responsive/product defect remains visible or blocks capture. Do not hide it with capture CSS.

## Camera And Delivery

Use `$LANDING_REPO/scripts/product-loop-camera.mjs` and `$LANDING_REPO/scripts/product-loop-encoder.mjs`. Their tests define dimensions, frame rate, maximum zoom, smoothness, loop reset, codecs, and poster quality. For semantic landing desktop stories, use `formFactor: "landing"` with `cameraProfile: "landing-editorial"` and cap maximum zoom at 2.0x. The `landing-editorial` profile requires both `focusTrack` and `pointerTrack`; configs without either are invalid. Native mobile stays native-size and uses the tested mobile cap, never more than 2.0x. Change these contracts test-first when the user requests a genuinely different delivery format.

Do not remove waits with cuts or speed changes. Improve the source choreography or recapture. One trim at the beginning/end is acceptable; time skips are not.

The camera is center-biased, not cursor-naive: follow the active semantic target while retaining the context needed to understand the action. Widen-pan-tighten for long journeys. Keep the full dialog or menu visible as a priority, even when that means less zoom. The visible cursor never leaves the frame, including its complete rendered glyph and safety margin.

Explicit rejection rules:

- Reject lazy global zoom that holds one crop regardless of the active target.
- Reject any zoom or camera move away from the active cursor while it is traveling.
- Reject stale UI, stale builds, accumulated rehearsal data, or captures from a non-current source checkout.
- Reject wide shots that make product text unreadable at actual size.

## Acceptance Gate

Follow [qa-checklist.md](references/qa-checklist.md). Do not ship until:

- raw and delivery dimensions are proven by decoded pixels, not only container metadata;
- cadence is constant 25 fps, timestamps have no gaps, the FFmpeg log proves zero duplicated and dropped frames, and no audio stream exists;
- 10/25/50/75/90% frames, an overview contact sheet, actual-size 100% frame proofs, and full playback pass visual review;
- camera motion is smooth, reaches intended depth, and uses the profile's tested loop frame: centered 1x for a wide loop, or identical first/settled/final crops for an approved focused landing/docs loop that improves readability while preserving identifying context;
- text, menus, diffs, pointer, and touch targets remain inside frame; frame-by-frame pointer containment passes with a deliberate edge margin;
- staged WebM, MP4, WebP, responsive source selection, lazy loading, and reduced-motion behavior pass codec probes and real-browser playback;
- SHA-256 hashes cover raw masters, camera configs, semantic metadata, proofs, and every delivery candidate;
- provenance records Kandev and landing commit SHAs, commands, source/delivery profiles, isolated ports/display/browser profile, and deterministic seed identity;
- teardown proves all capture processes, ports, X displays, browser profiles, temporary specs, databases, and temp data are gone.

Report story, seed, form factors, raw/delivery dimensions, durations, codecs, camera profile, output sizes/hashes, visual audit, browser checks, teardown, and any unsupported or blocked surface.
