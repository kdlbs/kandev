# Camera And Encoding

## Keep Camera Reversible

Keep camera motion out of raw capture. An unzoomed, full-frame native master supports alternate focus, different pacing, new poster timing, and future encoders without reseeding. Bake only the intentional pointer/touch treatment into raw pixels.

A reversible camera delivery always retains:

- the untouched continuous 1x raw master;
- dense semantic pointer/touch metadata and its SHA-256;
- the exact JSON camera config and its SHA-256;
- source and landing commit SHAs;
- the encoder command and resulting WebM, MP4, and WebP hashes.

Never overwrite or discard raw/config evidence after encoding.

## Tested Landing Profiles

Use the already-resolved `$LANDING_REPO`, then verify and run the focused tests for:

```text
scripts/product-loop-camera.mjs
scripts/product-loop-camera.test.mjs
scripts/product-loop-encoder.mjs
scripts/product-loop-encoder.test.mjs
```

Authoritative product-film contract:

- 25 fps constant cadence;
- general desktop delivery 2560x1600 from at least 3840x2400 source;
- when the checked-out landing tests recognize `cameraProfile: "landing-editorial"` with `formFactor: "landing"`, editorial delivery 1920x1200 from at least 3840x2400 source, with a 2x focused camera, VP9 CRF 26, and H.264 CRF 18;
- mobile delivery 1290x2796 from native 1290x2796 source;
- general desktop and focused documentation clips reach 1.50x; editorial landing clips reach 2x; native mobile reaches 1.18x;
- a wide loop starts, settles, and ends at centered 1x;
- an opt-in focused docs or landing loop has identical first, settled penultimate, and final camera frames;
- the final loop frame settles for at least 240ms;
- cosine-eased piecewise motion passes per-frame pan/zoom limits;
- one boundary trim, no concat and no speed-ramp `setpts`;
- muted VP9 WebM, H.264 MP4 with fast start, and WebP poster.

Treat the landing tests as executable source of truth. If a requested output differs, change those tests first; do not revive a legacy desktop profile in an ad hoc config.

Before using the editorial landing profile, confirm the focused tests recognize `cameraProfile: "landing-editorial"`; do not encode against an older landing checkout that lacks this profile. `landing-editorial` requires both `focusTrack` and `pointerTrack`, so every delivery carries explicit semantic framing evidence.

## Semantic Camera Design

Build keyframes from the recorded semantic story and complete pointer journey:

1. Establish a readable identifying frame.
2. Bias camera focus toward the active target and pointer while retaining enough surrounding UI to explain the action.
3. Hold context briefly, ignore micro-jitter, and sample every intentional movement for containment.
4. Move focus only when semantic story focus changes.
5. Prioritize a full dialog, menu, provider panel, sheet, or diff over a tighter crop.
6. Ease back to the configured loop frame and hold it long enough for a calm reset.

This is center-biased tracking, not mechanical hotspot centering. Use normalized centers from target bounds and target glyph bounds. Keep complete UI context visible while the camera follows the active pointer in the same direction.

For long travel, use widen-pan-tighten choreography:

1. Widen before movement until both the departure and destination regions fit.
2. Begin the pan before or with the active pointer, never after it has left the crop.
3. Pan with the pointer toward its semantic target.
4. Tighten only after arrival and after confirming the full destination menu/dialog remains visible.

Pass normalized dense `pointerTrack` waypoints and `pointerSafeMargin` to `createCameraTrack`. Derive the margin from the complete rendered cursor/touch glyph bounds around its hotspot, including asymmetric orientation near edges. Validate every output frame and every visibility interval. A failed containment check blocks delivery: the visible cursor never leaves the frame.

Use `loopFrame: "focused"` only with the `docs` or `landing` form factor. The opening, settled penultimate, and final camera keyframes must be identical, and the crop must still show enough context to identify the feature. This is an editorial framing tool, not a way to hide a product defect or misleading state. Native mobile media keeps its standard loop contract.

For a landing film, compose for the real theater rather than the master canvas. Review important frames around 760-950 CSS pixels wide. Labels, code, comments, and results must remain readable there. Start on the first meaningful subject, widen around that subject before a long cursor journey, pan while wide, and tighten at the destination. Do not repeatedly return to a full-workspace view merely to prove the product has navigation chrome.

## Explicit Camera Rejections

- Reject lazy global zoom that holds one magnified crop through unrelated actions.
- Reject any zoom or camera pan away from the active cursor while it moves.
- Reject camera motion designed from click timestamps without dense intermediate pointer samples.
- Reject a crop that keeps only the hotspot while clipping cursor glyph bounds.
- Reject a tight pan begun after the pointer is already outside the crop.
- Reject stale UI or an encode whose raw master lacks current-source provenance.
- Reject wide shots that make product text unreadable at actual size.
- Reject cropping or zoom used to hide incomplete menus, provider panels, dialogs, or product defects.

An editorial landing config uses the same matched-loop rule with `formFactor: "landing"`, a 3840x2400 source, and a 1920x1200 delivery. At 2x, the source crop is exactly 1920x1200, so the encoder does not invent detail by upscaling it. Add intermediate wide keyframes around travel instead of combining a deep zoom and long pan in one segment.

Minimal editorial-landing config shape. Replace the placeholder coordinates with values from the recorded pointer journey, and add the story keyframes between the matching opening and settled pair:

```jsonc
{
  "slug": "landing-<feature>",
  "rawPath": "<capture-root>/raw/desktop-<feature>.mp4",
  "outputDir": "<capture-root>/delivery",
  "trimStartMs": 0,
  "posterAtMs": 0,
  "sourceWidth": 3840,
  "sourceHeight": 2400,
  "outputWidth": 1920,
  "outputHeight": 1200,
  "camera": {
    "cameraProfile": "landing-editorial",
    "durationMs": 8000,
    "formFactor": "landing",
    "loopFrame": "focused",
    "pointerSafeMargin": 0.08,
    "pointerTrack": [
      { "tMs": 0, "x": 0.5, "y": 0.5 },
      { "tMs": 8000, "x": 0.5, "y": 0.5 }
    ],
    "focusTrack": [
      { "tMs": 0, "label": "opening feature context", "x": 0.5, "y": 0.5 },
      // Story focus targets follow the recorded semantic evidence.
      { "tMs": 7760, "label": "settled feature context", "x": 0.5, "y": 0.5 },
      { "tMs": 8000, "label": "matched loop frame", "x": 0.5, "y": 0.5 }
    ],
    "keyframes": [
      { "tMs": 0, "zoom": 2, "x": 0.5, "y": 0.5 },
      // Story keyframes follow the recorded pointer journey.
      { "tMs": 7760, "zoom": 2, "x": 0.5, "y": 0.5 },
      { "tMs": 8000, "zoom": 2, "x": 0.5, "y": 0.5 }
    ]
  }
}
```

## Landing Config Example

Use a landing slug with the required desktop prefix. Replace `<capture-root>` with the resolved staging root. Story keyframes below are illustrative; derive their coordinates and times from semantic metadata and run the tested smoothness/containment validators.

```json
{
  "slug": "desktop-integration-create-task",
  "rawPath": "<capture-root>/raw/desktop-integration-create-task.mp4",
  "outputDir": "<capture-root>/delivery",
  "trimStartMs": 480,
  "posterAtMs": 10000,
  "sourceWidth": 3840,
  "sourceHeight": 2400,
  "outputWidth": 1920,
  "outputHeight": 1200,
  "camera": {
    "cameraProfile": "landing-editorial",
    "durationMs": 12000,
    "formFactor": "landing",
    "loopFrame": "focused",
    "pointerSafeMargin": {
      "top": 0.04,
      "right": 0.06,
      "bottom": 0.05,
      "left": 0.04
    },
    "pointerTrack": [
      { "tMs": 0, "x": 0.5, "y": 0.5 },
      { "tMs": 1200, "x": 0.5, "y": 0.5 },
      { "tMs": 2200, "x": 0.18, "y": 0.43 },
      { "tMs": 3800, "x": 0.75, "y": 0.38 },
      { "tMs": 6800, "x": 0.75, "y": 0.38 },
      { "tMs": 8000, "x": 0.55, "y": 0.61 },
      { "tMs": 10000, "x": 0.55, "y": 0.61 },
      { "tMs": 11700, "x": 0.5, "y": 0.5 },
      { "tMs": 12000, "x": 0.5, "y": 0.5 }
    ],
    "focusTrack": [
      { "tMs": 0, "label": "integration context", "x": 0.5, "y": 0.5 },
      { "tMs": 2200, "label": "GitHub integration", "x": 0.18, "y": 0.43 },
      { "tMs": 6800, "label": "pull request row", "x": 0.65, "y": 0.48 },
      { "tMs": 10000, "label": "created task", "x": 0.55, "y": 0.61 },
      { "tMs": 11700, "label": "settled integration context", "x": 0.5, "y": 0.5 },
      { "tMs": 12000, "label": "matched loop frame", "x": 0.5, "y": 0.5 }
    ],
    "keyframes": [
      { "tMs": 0, "zoom": 1.25, "x": 0.5, "y": 0.5 },
      { "tMs": 500, "zoom": 1.25, "x": 0.5, "y": 0.5 },
      { "tMs": 1200, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 3800, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 6800, "zoom": 2, "x": 0.65, "y": 0.48 },
      { "tMs": 9800, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 10000, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 11700, "zoom": 1.25, "x": 0.5, "y": 0.5 },
      { "tMs": 12000, "zoom": 1.25, "x": 0.5, "y": 0.5 }
    ]
  }
}
```

For a wide loop, use centered 1x for the opening, settled penultimate, and final keyframes while still reaching the tested maximum during the story. For a focused landing loop, those same three frames must be exactly identical and retain enough context to identify the feature. Mobile uses a wide loop and native `1290x2796` dimensions.

Encode only to staging:

```bash
cd "$LANDING_REPO"
node scripts/product-loop-encoder.mjs "$CAPTURE_ROOT/configs/<slug>.json"
```

The encoder probes source duration and rejects an overrun. It writes `<slug>.webm`, `<slug>.mp4`, and `<slug>.webp` from one camera timeline.

## Avoid Awkward Motion

Jitter usually comes from too many keyframes, short segment durations, pointer-perfect tracking, or camera changes on the same frame as UI transitions. Fix by reducing targets, lengthening easing, and focusing on semantic regions. Cursor containment does not require centering every frame; it requires keeping the intentional journey within the safe crop.

Time skips usually come from concatenating holds, removing waits, or speeding mock-agent gaps. Fix source timing and recapture. Do not disguise them with crossfades.

Deeper zoom is useful only when content remains legible and contextual. Desktop can tolerate more lateral movement; mobile should mostly preserve center and use smaller zoom because sheets and bottom navigation already consume space.

A camera can pass pointer containment and still fail editorially. Reject tracks that mechanically center each click, oscillate between wide and tight without a change in subject, spend the opening on unused workspace, or make the viewer search for the action. Each move needs a narrative reason: establish, follow, reveal, compare, or settle.

## Alternate Deliveries

- Framing change: edit the camera config and re-encode from the same raw.
- Poster change: move `posterAtMs`; choose a pointer-free settled state.
- Static crop: derive from the full-resolution raw/poster; preserve the source.
- New aspect ratio: add a named tested profile. Do not stretch existing output.
- Long walkthrough: add a separate profile/player treatment. Do not weaken short-loop tests.
- GIF: derive after approval; keep WebM/MP4 canonical.

Never copy candidates into production merely because encoding passed. Keep them staged for side-by-side and actual-size review; promote only when separately authorized.
