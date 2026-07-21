# Product Media QA

## 1. Source And Isolation Contract

- [ ] Proven clean worktree SHA equals the freshly fetched `origin/main`; no alternate revision is eligible for product capture.
- [ ] Build/rehearsal frames show current UI and the requested theme without DOM/CSS patching.
- [ ] Unique temp home, database, repository root, ports, X display, and browser profile are documented.
- [ ] No developer instance, developer database, credentials, or production port was used.
- [ ] Final take starts from fresh/reset deterministic data with no rehearsal-created duplicates.
- [ ] Desktop and mobile use separate native scripts and raw masters.
- [ ] Desktop source is `3840x2400` at 25 fps and delivery is `1920x1200` at 25 fps.
- [ ] Mobile source and delivery are native `1290x2796` at 25 fps.
- [ ] Each raw master is one continuous take at 1x: no internal cuts, concat, camera, crop, speed ramps, or audio.
- [ ] OS cursor is disabled and exactly one DOM cursor/touch treatment is visible.
- [ ] DOM overlay and real browser pointer advance in lockstep through every eased travel sample; no independent overlay animation followed by a final browser-pointer move is allowed.
- [ ] A focused travel test proves zero endpoint displacement when real input settles, and normal-speed raw playback shows no pointer stepping, lag, or teleport.
- [ ] Re-sync the current pointer after direct setup input and before RECORD so the first semantic travel starts from the real browser position.
- [ ] The final RECORD trusted-event ledger, measured under active X11 capture load, keeps every meaningful travel at p95 <= 56ms and maximum <= 64ms; rehearsal-only cadence is not acceptance evidence.
- [ ] Dense semantic metadata includes timestamps, intermediate samples, target bounds, target glyph bounds, pointer glyph bounds, and visibility intervals. Target and pointer geometry are independently measured, never aliases; every interval begins at departure and includes motion, arrival, and settle.

## 2. Technical And Codec Probe

Use `ffprobe` on every raw, WebM, and MP4:

```bash
ffprobe -v error \
  -show_entries stream=index,codec_name,codec_type,width,height,r_frame_rate,avg_frame_rate,time_base,pix_fmt:format=duration,size \
  -of json <video>

ffprobe -v error -select_streams v:0 \
  -show_entries frame=best_effort_timestamp,best_effort_timestamp_time,pkt_duration,pkt_duration_time \
  -of csv=p=0 <video>
```

Verify:

- decoded dimensions match the exact source/delivery profile;
- `r_frame_rate` and `avg_frame_rate` are constant 25 fps;
- integer frame timestamps advance by the expected 25 fps interval expressed in the stream `time_base`, within one integer tick of rounding tolerance, with no duplicate, negative, or missing-frame gap; seconds fields are reporting aids, not the cadence authority;
- the full FFmpeg log reports zero duplicated and dropped frames and proves measured realtime recorder capacity at the exact source profile;
- story marks and camera timeline fit inside source duration;
- no audio stream exists in raw or deliveries;
- WebM is VP9 and MP4 is H.264 with fast start;
- WebP dimensions match delivery and its selected frame is settled;
- decoded first/middle/final raw frames fill all four edges without padded CSS-size content.

Save machine-readable probe output per slug under staged `qa/codec/`. Container metadata alone is not proof: decode pixels.

## 3. Camera And Pointer Audit

- [ ] Semantic desktop config uses `formFactor: "landing"` with `cameraProfile: "landing-editorial"`, `focusTrack`, and `pointerTrack`; maximum zoom is 2.0x.
- [ ] Mobile config uses `formFactor: "mobile"`; its tested cap is respected and never exceeds 2.0x.
- [ ] Camera keyframes derive from semantic metadata rather than guessed click positions.
- [ ] Camera stays center-biased toward the active pointer/target and never moves away from an active pointer journey.
- [ ] One smooth establishing tighten reaches a stable working depth; no zoom breathing occurs while the semantic subject remains unchanged.
- [ ] Long travel uses widen-pan-tighten: widen before departure, pan with motion, tighten after arrival.
- [ ] Every semantic camera move lasts at least 1.2 seconds (exception: the pointer is already settled inside one focus region), readable holds last 0.9-1.5 seconds, and routine pan median/p95 stay at or below 0.11 source-widths per second; a single pan peak may be higher only for a declared long journey that still passes the tested profile cap.
- [ ] Full menus, provider panels, sheets, dialogs, and diffs remain visible as framing priority.
- [ ] Frame-by-frame containment includes the full cursor/touch glyph and edge-safe margin for every visibility interval; the cursor never leaves the frame.
- [ ] Wide shots remain readable at actual display size.
- [ ] Every meaningful glyph measures at least 9px in the actual landing stage.
- [ ] Loop starts/settles/ends on its tested identical frame and holds at least 240ms.
- [ ] Raw master plus config can reproduce the delivery, proving a reversible camera.

Reject lazy global zoom, cursor-opposed camera motion, clipped glyphs, tight panning after departure, or any camera crop used to hide a product defect.

- no pointer teleport, duplicate pointer, or click before arrival;
- no state jump, cut, speed-up, dead wait, blank beat, or loader hold;
- camera reaches useful depth without oscillation;
- every intentional pointer/touch journey stays inside the camera crop with its configured edge-aware glyph margin at every encoded frame;
- camera remains within safe bounds and returns to its tested loop frame: centered 1x by default, or exactly matching first, settled penultimate, and final frames for an explicitly focused docs or landing clip;
- loop reset is calm rather than a snap;
- readable copy remains stable long enough to understand.

For an editorial landing film, repeat the timeline audit inside the production theater at 760px, 850px, and 950px CSS widths, including the current 964x602 actual landing stage. Reject a technically sharp delivery when the feature label, active control, changed line, feedback, or result cannot be read at those sizes. Reject compression or scaling artifacts that make text, glyphs, or labels hard to read in the actual player.

## 4. Actual-Size Visual Audit

Extract full-resolution frames at 10%, 25%, 50%, 75%, and 90%, plus every menu/dialog/provider-panel peak and the settled loop frame.

Inspect representative originals at actual-size 100% display scale. Record reviewer, monitor scale, file, and result. A contact sheet is required for timing/context comparison, but a downscaled contact sheet never substitutes for actual-size text, cursor, and clipping inspection.

Watch each complete loop at normal speed and 0.5x. Check:

- no pointer teleport, duplicate pointer, click before arrival, or cursor leaving frame;
- no state jump, cut, speed-up, dead wait, blank beat, loader hold, or stale UI;
- stable readable copy holds long enough to understand;
- no duplicate fixtures or tasks accumulated from rehearsal;
- no fixture, mock, test, E2E, slash-directive, local-path, localhost, host-username, browser-status, automation-banner, or developer-tooling artifacts;
- no fake provider, executor, or integration controls;
- organization, repository, task, branch, PR/issue, provider, and created task stay narratively consistent;
- titles, menus, provider panels, dialogs, sheets, diffs, checks, and primary actions are complete.

For browser-managed chrome and banners, inspect the perimeter from every decoded raw frame in frame-indexed contact sheets. Fixed-fraction samples and normal-speed playback are supplementary, not frame-complete evidence.

If text is only legible when zooming the proof viewer beyond 100%, reject the shot as unreadable.

## 5. Browser And Multi-Format QA

Keep candidates in staged delivery paths. In a real built player or dedicated staging page, test:

- VP9 WebM source load, metadata, play, loop, and final settled reset;
- H.264 MP4 fallback load, metadata, play, loop, and final settled reset;
- WebP poster decode and correct pre-playback/reduced-motion rendering;
- desktop selects `1920x1200` media and mobile selects its native `1290x2796` media;
- muted, inline, lazy/in-view loading without layout shift;
- `prefers-reduced-motion` keeps a meaningful poster and avoids autoplay motion;
- no `cover` crop hides UI and browser console/network show no media error;
- browser-reported dimensions/duration agree with codec probes.

Capture browser screenshots/logs and codec support/playback results under `qa/browser/`. Passing FFmpeg encode alone is not browser QA; passing one source alone is not multi-format QA.

Run the resolved landing repository's focused camera/encoder tests before encoding. Before an editorial landing film, confirm those tests recognize `formFactor: "landing"`; stop if the checkout has only the general desktop profile. Run broader landing checks only when integrating or promoting; staged-candidate capture must not modify production assets.

## 6. Hashes And Provenance

- [ ] Generate SHA-256 for every raw master, semantic metadata file, camera config, WebM, MP4, WebP, actual-size proof, contact sheet, and browser/codec QA record.
- [ ] Store deterministic seed identity and provider fixture versions.
- [ ] Store Kandev and landing commit SHAs and clean-worktree evidence.
- [ ] Store source/delivery profiles, FPS, DPR/CSS viewport, durations, codecs, sizes, crop/camera profile, and loop mode.
- [ ] Store capture/encode/probe commands plus resolved X display, ports, and isolated browser profile.
- [ ] Distinguish accepted files from rejected attempts and record rejection reasons.
- [ ] Keep raw, proof, config, and delivery candidates outside production assets.

- [ ] Compare old/new loops side by side.
- [ ] If a docs or landing clip uses a focused loop frame, confirm the first, settled penultimate, and final crops match exactly; use focused framing only when it improves readability or removes irrelevant chrome or fixture-only detail while preserving identifying context.
- [ ] Do not delete previous accepted media until replacement passes build and browser smoke.

Promotion is a separate authorization. Do not overwrite or copy landing assets during capture-only work.

## 7. Teardown

- [ ] Stop FFmpeg, Chrome, Playwright, frontend, backend, and Xvfb.
- [ ] Confirm every allocated backend/web/CDP port has no listener.
- [ ] Confirm the unique X socket/display is gone.
- [ ] Remove temporary E2E spec copies and browser profiles.
- [ ] Remove disposable executor profiles, database, repo, temp home, and detached capture worktree after needed evidence is staged.
- [ ] Check Kandev and landing worktrees; preserve unrelated user/agent changes.
- [ ] Record resolved targets and teardown proof in provenance.

Final report lists tests passed, checks not run, accepted and rejected artifact paths, hashes, browser/codec results, actual-size review, and teardown. Report the skill commit separately from capture delivery paths.
