# Product Media QA

## 1. Source Contract

- [ ] Disposable seed and ports documented.
- [ ] Desktop and mobile have separate scripts and raw masters.
- [ ] Raw is continuous 1x with no crop, camera, concat, speed change, or internal cut.
- [ ] OS cursor disabled; exactly one DOM cursor/touch treatment visible.
- [ ] Semantic events include timestamps, target bounds, and smooth motion samples.
- [ ] Every visible pointer/touch waypoint is normalized against the camera source and has an explicit visibility interval.
- [ ] Poster captured after recorder stops with pointer/touch hidden.

## 2. Technical Probe

Use `ffprobe` on every raw and delivery file:

```bash
ffprobe -v error \
  -show_entries stream=codec_name,width,height,r_frame_rate,avg_frame_rate:format=duration,size \
  -of json <video>

ffprobe -v error -select_streams v:0 \
  -show_entries frame=best_effort_timestamp_time,pkt_duration_time \
  -of csv=p=0 <video>
```

Verify:

- decoded dimensions match profile;
- `r_frame_rate` and `avg_frame_rate` are constant 25 fps;
- consecutive decoded frame timestamps advance by 0.04 seconds within one stream time-base tick, with no duplicate, negative, or missing-frame gaps;
- duration matches marked story and camera timeline;
- WebM codec is VP9, MP4 codec is H.264;
- no audio stream exists;
- poster dimensions match delivery.

Decode at least one raw frame and inspect all four edges. A 3840x2400 container with 1920x1200 active content and neutral padding is not high resolution.

## 3. Timeline Audit

Extract full-resolution frames at 10%, 25%, 50%, 75%, and 90%. Build a contact sheet for quick comparison, but inspect original frames when text appears truncated; downscaled mosaics can create false clipping.

Watch the complete loop at normal speed and at 0.5x. Check:

- no pointer teleport, duplicate pointer, or click before arrival;
- no state jump, cut, speed-up, dead wait, blank beat, or loader hold;
- camera reaches useful depth without oscillation;
- every intentional pointer/touch journey stays inside the camera crop with its configured edge-aware glyph margin at every encoded frame;
- camera remains within safe bounds and returns to its tested loop frame: centered 1x by default, or exactly matching first, settled penultimate, and final frames for an explicitly focused docs or landing clip;
- loop reset is calm rather than a snap;
- readable copy remains stable long enough to understand.

For an editorial landing film, repeat the timeline audit inside the production theater at 760px, 850px, and 950px CSS widths. Reject a technically sharp delivery when the feature label, active control, changed line, feedback, or result cannot be read at those sizes.

## 4. Content Audit

Search every sampled frame and poster for:

- fixture, mock, test, or E2E labels;
- `/tmp`, `/home`, local repository paths, localhost URLs, or host username;
- generic generated response or slash directive;
- fake provider/executor/integration controls;
- inconsistent organization, repository, task, branch, PR, or file names;
- clipped title, popover, menu, stage label, diff, composer, checks, or action;
- accidental notification, browser status text, or developer tooling.

Confirm visible implementation, diff, test, and review claims are supported by seeded repository/provider state.

## 5. Landing Integration

Production bundle per slug:

```text
public/product/loops/<slug>.webm
public/product/loops/<slug>.mp4
public/product/loops/<slug>.webp
```

Verify player behavior:

- WebM primary and MP4 fallback both load;
- poster appears before playback and under reduced motion;
- video is muted, inline, lazy/in-view loaded, and does not shift layout;
- only active desktop/mobile story loads;
- native mobile media uses its own aspect ratio and source;
- controls, tabs, and carousel semantics remain keyboard-accessible;
- no `cover` crop hides important UI.

Run landing checks only from the resolved landing repository root:

```bash
cd "$LANDING_REPO"
pnpm exec vitest run scripts/product-loop-camera.test.mjs scripts/product-loop-encoder.test.mjs
pnpm test
pnpm exec tsc --noEmit
pnpm run build:pages
```

Smoke-check desktop, mobile, and `prefers-reduced-motion` in a real browser. Confirm media requests return 200/206 and browser console has no errors.

## 6. Promotion And Provenance

- [ ] Encode to staging first.
- [ ] Compare old/new loops side by side.
- [ ] Promote only approved WebM/MP4/WebP files.
- [ ] If a docs or landing clip uses a focused loop frame, confirm the first, settled penultimate, and final crops match exactly; use focused framing only when it improves readability or removes irrelevant chrome or fixture-only detail while preserving identifying context.
- [ ] Record SHA-256, dimensions, duration, codec, size, source seed, and capture command.
- [ ] Keep raw/proof bundle outside production assets unless repository policy requests it.
- [ ] Do not delete previous accepted media until replacement passes build and browser smoke.

## 7. Teardown

- [ ] Stop FFmpeg, Chrome, Playwright, backend, frontend, and Xvfb.
- [ ] Confirm capture backend/web/CDP ports have no listeners.
- [ ] Remove temporary E2E spec copies and browser profiles.
- [ ] Remove disposable executor profiles, database, repo, and temp home.
- [ ] Check both Kandev and landing worktrees; preserve unrelated user/agent changes.

Final report must distinguish tests that passed, checks not run, and any product surface blocked by a real UI defect.
