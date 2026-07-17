# Camera And Encoding

## Keep Camera Out Of Raw Capture

An unzoomed native master supports alternate focus, deeper/shallow zoom, new poster timing, changed landing composition, and future encoders without reseeding. Bake only the intentional pointer/touch treatment into raw pixels.

## Use Landing's Tested Pipeline

Use the already-resolved `$LANDING_REPO`, then verify these paths within it:

```text
scripts/product-loop-camera.mjs
scripts/product-loop-camera.test.mjs
scripts/product-loop-encoder.mjs
scripts/product-loop-encoder.test.mjs
```

Run focused tests from `$LANDING_REPO` before encoding. Current contract:

- 25 fps constant cadence;
- desktop delivery 2560x1600 from at least 3840x2400 source;
- mobile delivery 1290x2796 from native 1290x2796 source;
- landing desktop and focused documentation clips reach 1.50x; native mobile reaches 1.18x;
- centered 1x opening and ending by default;
- an opt-in docs-only focused loop frame when the first, settled penultimate, and final camera frames match exactly;
- at least 240ms on the settled loop frame before reset;
- cosine-eased piecewise motion with per-frame smoothness checks;
- one trim, no concat, no speed-ramp `setpts`;
- muted VP9 WebM, H.264 MP4 with fast start, and WebP poster.

Treat tests as source of truth if these values evolve.

## Camera Design

Design keyframes from semantic story events and the recorded pointer journey:

1. Start centered at 1x unless a short docs clip intentionally uses one matched focused loop frame to exclude irrelevant chrome or fixture-only detail.
2. Hold context briefly.
3. Ease toward the first important target while the pointer travels.
4. Hold or drift gently across related actions. Ignore micro-jitter, but never let intentional pointer travel leave the crop.
5. Move focus only when story focus changes materially.
6. Ease back to the opening loop frame before the final settled hold.

Use normalized centers from target bounds. Keep full menus/dialogs/diffs inside the crop at maximum zoom. Optical focus may differ from exact pointer center when surrounding context is important.

Pass normalized `pointerTrack` waypoints and a `pointerSafeMargin` to `createCameraTrack`. The camera module checks every output frame, not only click timestamps. Derive the margin from the complete rendered glyph around its hotspot; an asymmetric `{ top, right, bottom, left }` map is valid when the pointer orientation requires it. A failed containment check blocks delivery. Do not claim a clipped source cursor is safe merely because its hotspot remains in frame.

For long journeys between distant regions, do not pan a tight crop after the pointer has already left. Ease out far enough to contain both endpoints, begin the pan before or with the intentional movement, then ease back into the destination. A wide transition is preferable to an unexplained off-screen pointer.

Use `loopFrame: "focused"` only with the `docs` form factor. The opening, settled penultimate, and final camera keyframes must be identical, and the crop must still show enough context to identify the feature. This is an editorial framing tool, not a way to hide a product defect or misleading state. Landing desktop/mobile media must keep the standard wide loop reset.

Example config shape. Replace `<capture-root>` with the unique resolved `CAPTURE_ROOT` before encoding:

```json
{
  "slug": "desktop-run-inspect",
  "rawPath": "<capture-root>/raw/desktop-run.mp4",
  "outputDir": "<capture-root>/delivery",
  "trimStartMs": 480,
  "posterAtMs": 7200,
  "sourceWidth": 3840,
  "sourceHeight": 2400,
  "outputWidth": 2560,
  "outputHeight": 1600,
  "camera": {
    "durationMs": 8400,
    "formFactor": "desktop",
    "pointerSafeMargin": 0.08,
    "pointerTrack": [
      { "tMs": 0, "x": 0.5, "y": 0.5 },
      { "tMs": 2700, "x": 0.62, "y": 0.38 },
      { "tMs": 5900, "x": 0.56, "y": 0.61 },
      { "tMs": 8400, "x": 0.5, "y": 0.5 }
    ],
    "keyframes": [
      { "tMs": 0, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 450, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 2700, "zoom": 1.5, "x": 0.57, "y": 0.43 },
      { "tMs": 5900, "zoom": 1.5, "x": 0.55, "y": 0.57 },
      { "tMs": 8100, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 8400, "zoom": 1, "x": 0.5, "y": 0.5 }
    ]
  }
}
```

Encode from landing repo:

```bash
cd "$LANDING_REPO"
node scripts/product-loop-encoder.mjs /path/to/config.json
```

The encoder probes source duration and rejects an overrun. It writes `<slug>.webm`, `<slug>.mp4`, and `<slug>.webp` from one camera timeline.

## Avoid Awkward Motion

Jitter usually comes from too many keyframes, short segment durations, pointer-perfect tracking, or camera changes on the same frame as UI transitions. Fix by reducing targets, lengthening easing, and focusing on semantic regions. Cursor containment does not require centering every frame; it requires keeping the intentional journey within the safe crop.

Time skips usually come from concatenating holds, removing waits, or speeding mock-agent gaps. Fix source timing and recapture. Do not disguise them with crossfades.

Deeper zoom is useful only when content remains legible and contextual. Desktop can tolerate more lateral movement; mobile should mostly preserve center and use smaller zoom because sheets and bottom navigation already consume space.

## Alternate Deliveries

- Framing change: edit camera config and re-encode from same raw.
- Poster change: move `posterAtMs`; keep pointer-free settled state.
- Static crop: derive from full-resolution raw/poster; preserve uncropped source.
- New aspect ratio: add a named tested delivery profile. Do not stretch existing output.
- Long walkthrough: add a separate profile and player treatment. Do not weaken short-loop tests globally.
- GIF: derive after approval with a palette-aware encoder; keep WebM/MP4 canonical.

Never overwrite approved media until side-by-side review passes. Encode to staging, compare, then promote selected files.
