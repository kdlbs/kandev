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
- desktop reaches 1.20x; mobile reaches 1.14x;
- centered 1x opening and ending;
- at least 240ms settled centered 1x before loop reset;
- cosine-eased piecewise motion with per-frame smoothness checks;
- one trim, no concat, no speed-ramp `setpts`;
- muted VP9 WebM, H.264 MP4 with fast start, and WebP poster.

Treat tests as source of truth if these values evolve.

## Camera Design

Design 4-7 keyframes from semantic story events:

1. Start centered at 1x.
2. Hold context briefly.
3. Ease toward the first important target while the pointer travels.
4. Hold or drift gently across related actions; do not chase every click.
5. Move focus only when story focus changes materially.
6. Ease back to centered 1x before the final settled hold.

Use normalized centers from target bounds. Keep full menus/dialogs/diffs inside the crop at maximum zoom. Optical focus may differ from exact pointer center when surrounding context is important.

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
    "keyframes": [
      { "tMs": 0, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 450, "zoom": 1, "x": 0.5, "y": 0.5 },
      { "tMs": 2700, "zoom": 1.2, "x": 0.57, "y": 0.43 },
      { "tMs": 5900, "zoom": 1.2, "x": 0.55, "y": 0.57 },
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

Jitter usually comes from too many keyframes, short segment durations, pointer-perfect tracking, or camera changes on the same frame as UI transitions. Fix by reducing targets, lengthening easing, and focusing on semantic regions.

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
