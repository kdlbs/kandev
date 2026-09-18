# Coordinator view evidence

Genuine Chromium/Pixel 5 captures from the disposable Kandev E2E backend on
2026-09-18 from clean source `69753564d0d7c0121e8681a72e5b4ce8c384a106`.
The database, browser context, tasks, conversations and execution
profile are fixture-owned. No production database, account, credential, prompt
or conversation was used. The mock provider is not evidence of real-provider
quality or assistant inspection safety.

The runtime is a byte-identical copy of the qualified candidate, with the
fixture-only mock worker placed beside it. Captures use the E2E web build of
that same source, including its pseudo locale. The embedded production web is
verified separately in the [candidate receipt](../../candidate-receipt.json).

- [Desktop task overview and chat](desktop.png)
- [Selected-coordinator filter and independent draft](selected-coordinator.png)
- [Mobile tasks, with the first row visible without scrolling](mobile-tasks.png)
- [Mobile conversation and generic draft](mobile-chat.png)
- [Silent typing preview](coordinator-demo.webm), VP9, 1440 × 1000, 1 fps,
  7 seconds. It shows typing a generic request while the task groups remain
  visible. This is a short interaction clip, not a narrated walkthrough.

The four screenshots and all seven source video frames were visually reviewed.
Only fictional task titles and the generic example requests are visible. The
video has no audio stream. [Artifact hashes](sha256.json) identify the shipped
files; [capture provenance](capture.json) records source/test/frame hashes and
the verified video stream/frame count.
No redaction overlay or image generation was used.

The source tests are
`apps/web/e2e/tests/orchestration/coordinator-view.spec.ts` and
`mobile-coordinator-view.spec.ts`. Enable `CAPTURE_PR_ASSETS=1` with the managed
runner to reproduce the captures. The capture helper records 5 PNG frames per
second; the video includes all frames at their original dimensions, played at
1 fps (five times slower) for readability:

```sh
ffmpeg -framerate 1 -i frame-%04d.png -c:v libvpx-vp9 -threads 1 \
  -b:v 0 -crf 24 -pix_fmt yuv420p -an coordinator-demo.webm
```

The implementation is based on exact v0.94.0
(`bf819a0228e742d069c528293d848c985a4d1bd1`), with task observations introduced in
`13a3a5ae5`. These refreshed captures accompany the completed
[assistant evidence](../../assistant-evidence.md). No new public comment, PR or
media publication was made.
