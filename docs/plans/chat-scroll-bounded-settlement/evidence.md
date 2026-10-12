# Issue 4100 Investigation Evidence

Date: 2026-10-08. Source commit: `6254b05eb0242b67900e160ff5b1d9acbb7962ad`.
The working tree was clean before investigation.

## Scope and confidence

The [issue](https://github.com/kdlbs/kandev/issues/4100) reports long-uptime CPU
and memory growth in the macOS desktop WebContent process. Its
[latest comment](https://github.com/kdlbs/kandev/issues/4100#issuecomment-6067024473)
contains native stack excerpts for forced layout from scroll metric reads.
It contains no attached profile or heap snapshot.

Confirmed: `createChatScrollMotion` can run indefinitely when a browser returns
a fractional position that cannot equal its integer target. The same termination
predicate exists in `v0.97.0` and the investigated commit.

Unconfirmed: this defect caused the reporter's macOS CPU or memory growth.
Linux WebKit did not reproduce the tested rounding cases. No macOS host or
day-long session was available. The native stack excerpt does not identify the
JavaScript caller or prove which stylesheet or animation owner retains memory.
This package must not close issue 4100 as fully resolved.

## Source findings

- `apps/web/components/task/chat/chat-scroll-motion.ts`: `tick` reads
  `scrollHeight`, `clientHeight`, and `scrollTop`. It writes an interpolated
  position and queues another frame whenever `scrollTop !== target`.
  Elapsed interpolation time does not terminate the original driver.
- `use-chat-scroll-motion.ts`: `canFollow` checks panel visibility. Effect
  cleanup disposes the driver and resize observer when the panel hides.
  Hidden-panel activity is therefore not established by this source trace.
  This is a panel guard, not a claim about every document-visibility case.
- `task-chat-panel.tsx`: non-Dockview previews have separate read visibility
  and transcript geometry visibility. A false read-visibility value alone
  does not prove that a transcript is hidden.
- `components/theme/app-theme.tsx`: the temporary transition stylesheet
  attaches on theme application and removes through a timeout. Its effect
  depends on theme inputs, not every chat update. No continual churn was reproduced.
- `components/grid-spinner.tsx`, `components/shared/chat-markdown-motion.tsx`,
  and `components/task/chat/chat-motion.tsx` contain animation cancellation
  paths. Their presence does not prove WebKit releases all engine resources.
- `components/task/vscode-panel.tsx` embeds code-server in an iframe.
  The reported 13 open panels remain relevant to a future native profile.
  This investigation does not attribute iframe resources to the host scroll driver.

## Reproduction results

The browser probe transpiles the real driver without modifying it. It uses
real browser geometry and a deterministic animation-frame clock. Each case
advances 600 callbacks slots, approximately ten simulated seconds.
This measures termination and read behavior, not CPU utilization or heap growth.

The matrix uses heights `200`, `200.25`, `200.5`, and `200.75` CSS pixels,
with CSS zoom values `1`, `1.1`, `1.25`, and `1.5`.
CSS zoom creates fractional readback. It is not an emulation of Tauri's native zoom API.

| Browser and source                | Cases | Still running after 600 slots | Maximum callbacks | Maximum bottom error |
| --------------------------------- | ----: | ----------------------------: | ----------------: | -------------------: |
| Chromium, original                |    16 |                             6 |               600 |          0.909119 px |
| Chromium, in-memory candidate     |    16 |                             0 |                11 |          0.909119 px |
| Linux WebKit, original            |    16 |                             0 |                11 |                 0 px |
| Linux WebKit, in-memory candidate |    16 |                             0 |                11 |                 0 px |

For Chromium at zoom `1.25` and height `200.5`, the target is `799`.
The browser returns `799.2000122070312`. The original driver remains active.
The candidate terminates with the same acceptable visible position.
The candidate was a string substitution in memory, not a production edit.

A temporary Vitest regression used `scrollHeight=1000`, `clientHeight=200`,
and a scroll setter clamped to `799.75`. After 600 slots, it recorded:

```text
{ top: 799.75, reads: 600, pending: 1, running: true }
AssertionError: expected true to be false
```

Command: `pnpm exec vitest run components/task/chat/chat-scroll-motion.test.ts components/task/chat/issue-4100-repro.test.ts`
from `apps/web`. Result: 17 existing tests passed, 1 temporary regression failed
for the expected reason. The temporary file was removed after diagnosis.

## Repeatable browser probe

Prerequisites: workspace dependencies, Chromium, WebKit, and their host libraries.
The investigation used Playwright 1.61.1 and its WebKit 26.5 build 2311 on Debian 13.
`pnpm exec playwright install webkit` installed the browser. The missing host
library came from `apt-get install -y libwoff1`.

Run from the repository root. Each document is isolated and contains no task data.
The probe closes every browser it starts. It writes no repository files.

```bash
(cd apps/web && node <<'NODE'
const fs = require('fs');
const ts = require('typescript');
const { chromium, webkit } = require('@playwright/test');
const actual = fs.readFileSync('components/task/chat/chat-scroll-motion.ts', 'utf8');
const candidate = actual.replace(
  'if (element.scrollTop !== target) frame = requestAnimationFrame(tick);',
  'if (progress < 1 && Math.abs(element.scrollTop - target) > 1) frame = requestAnimationFrame(tick);',
);
(async () => {
  for (const [engine, type] of Object.entries({ chromium, webkit })) {
    const browser = await type.launch({ headless: true });
    try {
      for (const [variant, source] of Object.entries({ actual, candidate })) {
        const page = await browser.newPage();
        await page.setContent('<div id="scroll" style="overflow:auto"><div style="height:1000px"></div></div>');
        const code = ts.transpileModule(source, {
          compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
        }).outputText;
        await page.addScriptTag({ content: 'var exports={};' + code });
        const result = await page.evaluate(() => {
          const el = document.getElementById('scroll');
          let failed = 0, maxFrames = 0, maxError = 0;
          for (const zoom of [1, 1.1, 1.25, 1.5]) {
            for (const height of [200, 200.25, 200.5, 200.75]) {
              el.style.zoom = zoom;
              el.style.height = height + 'px';
              el.scrollTop = 0;
              const frames = new Map();
              let id = 0, ticks = 0;
              window.requestAnimationFrame = cb => { frames.set(++id, cb); return id; };
              window.cancelAnimationFrame = key => frames.delete(key);
              const motion = exports.createChatScrollMotion(el, () => true, () => {});
              motion.request();
              for (let n = 1; n <= 600; n++) {
                const pending = [...frames.values()];
                frames.clear();
                for (const cb of pending) { ticks++; cb(n * 16.67); }
              }
              if (motion.isRunning()) failed++;
              maxFrames = Math.max(maxFrames, ticks);
              maxError = Math.max(maxError, Math.abs(el.scrollTop - (el.scrollHeight - el.clientHeight)));
              motion.dispose();
            }
          }
          return { cases: 16, failed, maxFrames, maxError };
        });
        console.log({ engine, variant, ...result });
        await page.close();
      }
    } finally {
      await browser.close();
    }
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
NODE
)
```

After implementation, the substitution can become a no-op because the old
predicate is absent. At that point, both variants must settle.

## Inspector question and remaining diagnosis

At the investigated source commit (`6254b05eb0242b67900e160ff5b1d9acbb7962ad`),
`apps/desktop/src-tauri/Cargo.toml` did not declare Tauri's `devtools` feature,
and no explicit inspector enablement was found in the desktop source. Tauri's
[debugging documentation](https://v2.tauri.app/develop/debug/#using-the-inspector-in-production)
states that production inspection requires feature enablement or a debug build.
This PR adds a native release inspector entry point through View > Developer
Tools and the platform shortcut. Native release interaction remains pending on
Linux, macOS, and Windows; the [desktop developer-tools work order](../desktop-developer-tools/task-01-enable-developer-tools.md)
records those checks, including Safari inspection and a heap snapshot on macOS
13.3 or later.

The remaining macOS investigation needs a profile before process termination,
with JavaScript caller attribution and stylesheet/animation ownership over time.
Counts across task, terminal, and editor mount/unmount cycles can distinguish
host resources from code-server documents. No speculative stylesheet removal or
global animation disablement belongs in this fix package.

## Cleanup

All probe browsers closed. The temporary Vitest file was removed. No isolated
Kandev backend was necessary or started. No live user instance was mutated.
Browser binaries and the required system library remain installed as development tools.
Only design and plan documents remain as workspace changes.
