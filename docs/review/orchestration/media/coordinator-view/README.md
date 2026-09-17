# Coordinator view evidence

Genuine Chromium/Pixel 5 captures from the disposable Kandev E2E backend on
2026-09-17. The database, browser context, tasks, conversations and execution
profile are fixture-owned. No production database, account, credential, prompt
or conversation was used. The mock provider is not evidence of real-provider
quality or assistant inspection safety.

- [Desktop task overview and chat](desktop.png)
- [Selected-coordinator filter and independent draft](selected-coordinator.png)
- [Mobile tasks, with the first row visible without scrolling](mobile-tasks.png)
- [Mobile conversation and generic draft](mobile-chat.png)
- [Silent typing preview](coordinator-demo.webm), VP9, 1440 × 1000, 5 fps,
  1.4 seconds. It shows typing a generic request while the task groups remain
  visible. This is a short interaction clip, not a narrated walkthrough.

The four screenshots and all seven source video frames were visually reviewed.
Only fictional task titles and the generic example requests are visible. The
video has no audio stream. `sha256.json` records the shipped artifact digests.
No redaction overlay or image generation was used.

The source tests are
`apps/web/e2e/tests/orchestration/coordinator-view.spec.ts` and
`mobile-coordinator-view.spec.ts`. Enable `CAPTURE_PR_ASSETS=1` with the managed
runner to reproduce the captures. The capture helper records 5 PNG frames per
second; the video is an unchanged-size VP9 encoding of those source frames:

```sh
ffmpeg -framerate 5 -i frame-%04d.png -c:v libvpx-vp9 -threads 1 \
  -b:v 0 -crf 24 -pix_fmt yuv420p -an coordinator-demo.webm
```

The implementation is based on exact v0.94.0
(`bf819a0228e742d069c528293d848c985a4d1bd1`), with task observations introduced in
`13a3a5ae5`. These captures accompany the central-view implementation commit;
they replace neither the original issue's prototype media nor the pending
assistant evidence. No new public comment, PR or media publication was made.
