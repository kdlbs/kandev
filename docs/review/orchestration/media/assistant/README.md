# Assistant interface evidence

Genuine Chromium/Pixel 5 captures from clean source
`c357f9ab94d3790a471d9db587c0a063c7f8d838` on 2026-09-18, above exact
Kandev v0.94.0. The disposable E2E Workspace contains only generic instructions,
example tasks and mock workers. No live database, conversation or provider log
was used for these assets.

- [Desktop native question](desktop-attention.png)
- [Desktop saved preference and original instruction](desktop-memory.png)
- [Phone native question and offered answers](mobile-attention.png)
- [Phone memory provenance close-up](mobile-memory.png)
- [Silent memory interaction](assistant-demo.webm): VP9, 1440 × 1000, five frames
  played at 1 fps for five seconds. The original 5 fps capture is slowed five
  times for readability; every source frame is included.

The central mock profile is intentionally unsupported for restricted assistant
execution. The visible banner reports that limitation. These assets demonstrate
human native-input and memory controls, not a model executing an assistant turn.
The separate [provider trial receipt](../../assistant-provider-trial.json) records
the real restricted-provider read and denied write.

All four screenshots and all five source video frames were visually inspected.
Only generic requests and disposable fixture identifiers are visible. The clip
has no audio. No redaction overlay or generated image was used.
[Artifact hashes](sha256.json) identify the shipped files;
[capture provenance](capture.json) records source/test/frame hashes and the
verified stream/frame count.

Reproduce with `CAPTURE_PR_ASSETS=1` and the managed E2E runner, one worker and
no retries. The source specs are `personal-assistant.spec.ts` and
`mobile-personal-assistant.spec.ts` under `apps/web/e2e/tests/orchestration/`.
Archive `.pr-assets` immediately after each invocation: a later Playwright run
cleans that output directory. Encode the inspected frame sequence with:

```sh
ffmpeg -framerate 1 -i frame-%04d.png -c:v libvpx-vp9 -threads 1 \
  -b:v 0 -crf 24 -pix_fmt yuv420p -an assistant-demo.webm
```

The desktop capture invocation passed all four coordinator/assistant/maintenance/
workspace-link tests; the phone invocation passed the corresponding four tests.
The desktop invocation was repeated after archiving phone assets to preserve both
output sets. This was capture housekeeping, with unchanged source and no retries.
These are private review artifacts; no additional public publication is implied.
