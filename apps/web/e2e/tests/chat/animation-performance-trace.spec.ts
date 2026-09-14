import type { CDPSession, Page, TestInfo } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

const TRACE_WINDOW_MS = 8_340;
const TARGET_PATTERN = /spinner-grid-cube|chat-input-glow-(?:running|starting)/;
const TARGET_SELECTORS = {
  grid: ".spinner-grid-cube",
  pulse: "[data-compositor-pulse]",
} as const;
const TRACE_REPEATS = parsePositiveInteger(process.env.KANDEV_E2E_ANIMATION_TRACE_REPEATS ?? "1");

type TraceEvent = {
  name?: string;
  args?: unknown;
  dur?: number;
};

type TraceMetric = {
  count: number;
  durationMs: number;
};

type TraceMetrics = {
  updateLayoutTree: TraceMetric;
  layerize: TraceMetric;
  layout: TraceMetric;
  paint: TraceMetric;
  runTask: TraceMetric;
  functionCall: TraceMetric;
  targetInvalidations: number;
  targetInvalidationsByGroup: Record<keyof typeof TARGET_SELECTORS, number>;
};

type NumericSummary = {
  min: number;
  median: number;
  max: number;
};

type TraceMetricsSummary = {
  updateLayoutTree: { count: NumericSummary; durationMs: NumericSummary };
  layerize: { count: NumericSummary; durationMs: NumericSummary };
  layout: { count: NumericSummary; durationMs: NumericSummary };
  paint: { count: NumericSummary; durationMs: NumericSummary };
  runTask: { count: NumericSummary; durationMs: NumericSummary };
  functionCall: { count: NumericSummary; durationMs: NumericSummary };
  targetInvalidations: NumericSummary;
  targetInvalidationsByGroup: Record<keyof typeof TARGET_SELECTORS, NumericSummary>;
};

type AnimationInventory = {
  css: Array<{
    target: string;
    pseudo: string | null;
    animationName: string;
  }>;
  webAnimations: Array<{
    target: string;
    constructor: string;
    playState: string;
    animationName: string | null;
  }>;
};

test.skip(
  process.env.KANDEV_E2E_ANIMATION_TRACE !== "1",
  "Run explicitly to capture the 8.34-second animation performance control.",
);
test.describe.configure({ retries: process.env.KANDEV_E2E_ANIMATION_TRACE === "1" ? 0 : 1 });

test("attributes steady animation work for compositor and CSS fallback paths", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(900_000);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Animation performance trace",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessage("/slow 300s");

  const glow = session.activeChat().getByTestId("chat-input-glow");
  await expect(glow).toBeVisible({ timeout: 30_000 });
  const dialog = await openQuickChatWithAgent(testPage);
  await sendQuickChatMessage(dialog, testPage, "/slow 300s");
  const cubes = dialog.locator(".spinner-grid-cube");
  await expect(cubes).toHaveCount(18);
  await expectTraceTargetsPresent(testPage);
  await expectCompositorTargets(testPage);

  const normalInventory = await inspectAnimationInventory(testPage);
  const normalRuns = await captureRepeatedTraces(testPage, testInfo, "normal-page", {
    disableScriptExecution: false,
  });

  const disableCssMotion = await testPage.addStyleTag({
    content: "*,*::before,*::after{animation:none!important;transition:none!important}",
  });
  await dwell(1_000, "clock-separation", "exclude one-time effect setup from the trace window");
  const compositor = await captureTrace(testPage, testInfo, "compositor-motion");
  expect(compositor.targetInvalidations).toBe(0);

  await disableCssMotion.evaluate((element) => element.remove());
  const removeCssFallback = await installCssFallbackControl(testPage);
  try {
    await dwell(
      1_000,
      "clock-separation",
      "exclude fallback style installation from the trace window",
    );
    const cssFallback = await captureRepeatedTraces(testPage, testInfo, "css-fallback-motion", {
      disableScriptExecution: true,
    });
    const cssFallbackSummary = summarizeMetrics(cssFallback);

    const isolatedRuns = await captureMotionIsolationArms(testPage, testInfo);
    const allMotionPaused = isolatedRuns.find((run) => run.arm === "all-motion-paused");
    expect(allMotionPaused).toBeDefined();
    expect(cssFallbackSummary.targetInvalidations.max).toBeGreaterThan(0);

    const cssFallbackBaseline = isolatedRuns.find((run) => run.arm === "css-fallback-baseline");
    expect(cssFallbackBaseline).toBeDefined();
    if (!cssFallbackBaseline) throw new Error("Missing script-enabled CSS fallback baseline");
    const cssFallbackBaselineSummary = summarizeMetrics(cssFallbackBaseline.metrics);
    for (const [arm, group] of [
      ["grid-paused", "grid"],
      ["pulse-paused", "pulse"],
    ] as const) {
      const pausedRun = isolatedRuns.find((run) => run.arm === arm);
      expect(pausedRun).toBeDefined();
      if (!pausedRun) throw new Error(`Missing ${arm} isolation run`);
      const pausedSummary = summarizeMetrics(pausedRun.metrics);
      expect(pausedSummary.targetInvalidationsByGroup[group].median).toBeLessThan(
        cssFallbackBaselineSummary.targetInvalidationsByGroup[group].median,
      );
    }
    const allMotionPausedSummary = summarizeMetrics(allMotionPaused!.metrics);
    for (const group of ["grid", "pulse"] as const) {
      expect(allMotionPausedSummary.targetInvalidationsByGroup[group].median).toBeLessThan(
        cssFallbackBaselineSummary.targetInvalidationsByGroup[group].median,
      );
    }

    const finalInventory = await inspectAnimationInventory(testPage);
    await testInfo.attach("animation-attribution-report.json", {
      body: Buffer.from(
        JSON.stringify(
          {
            repeats: TRACE_REPEATS,
            normalInventory,
            finalInventory,
            normalRuns,
            normalSummary: summarizeMetrics(normalRuns),
            compositor,
            cssFallback,
            cssFallbackSummary,
            cssFallbackBaselineSummary,
            isolatedRuns,
            isolatedSummaries: isolatedRuns.map((run) => ({
              arm: run.arm,
              summary: summarizeMetrics(run.metrics),
            })),
          },
          null,
          2,
        ),
      ),
      contentType: "application/json",
    });
    console.info(
      `[animation-performance-trace] ${JSON.stringify({
        repeats: TRACE_REPEATS,
        normalRuns,
        normalSummary: summarizeMetrics(normalRuns),
        compositor,
        cssFallback,
        cssFallbackSummary,
        cssFallbackBaselineSummary,
        isolatedRuns,
        isolatedSummaries: isolatedRuns.map((run) => ({
          arm: run.arm,
          summary: summarizeMetrics(run.metrics),
        })),
        normalInventory,
        finalInventory,
      })}`,
    );
  } finally {
    await removeCssFallback();
  }
});

type TraceCaptureOptions = {
  disableScriptExecution?: boolean;
};

type IsolationArm = "css-fallback-baseline" | "grid-paused" | "pulse-paused" | "all-motion-paused";

type IsolationRun = {
  arm: IsolationArm;
  metrics: TraceMetrics[];
};

async function captureRepeatedTraces(
  page: Page,
  testInfo: TestInfo,
  label: string,
  options: TraceCaptureOptions,
): Promise<TraceMetrics[]> {
  const metrics: TraceMetrics[] = [];
  for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
    await expectTraceTargetsPresent(page);
    metrics.push(await captureTrace(page, testInfo, `${label}-${repeat}`, options));
  }
  return metrics;
}

async function expectTraceTargetsPresent(page: Page) {
  await expect(page.locator(TARGET_SELECTORS.grid)).toHaveCount(18);
  await expect(page.locator(TARGET_SELECTORS.pulse)).toHaveCount(1);
}

async function captureMotionIsolationArms(page: Page, testInfo: TestInfo): Promise<IsolationRun[]> {
  const arms: Array<{ arm: IsolationArm; selectors: string[] }> = [
    { arm: "css-fallback-baseline", selectors: [] },
    { arm: "grid-paused", selectors: [TARGET_SELECTORS.grid] },
    { arm: "pulse-paused", selectors: [TARGET_SELECTORS.pulse] },
    { arm: "all-motion-paused", selectors: [] },
  ];
  const runs: IsolationRun[] = [];
  for (const { arm, selectors } of arms) {
    await expectTraceTargetsPresent(page);
    const restore =
      arm === "css-fallback-baseline"
        ? async () => undefined
        : await pauseMotion(page, selectors, arm === "all-motion-paused");
    try {
      if (arm === "css-fallback-baseline") {
        await expectCssFallbackTargets(page);
      } else {
        await expectMotionPaused(page, selectors, arm === "all-motion-paused");
        if (arm === "grid-paused") {
          await expectCssFallbackGroupRunning(page, TARGET_SELECTORS.pulse);
        } else if (arm === "pulse-paused") {
          await expectCssFallbackGroupRunning(page, TARGET_SELECTORS.grid);
        }
      }
      const metrics = await captureRepeatedTraces(page, testInfo, arm, {
        disableScriptExecution: false,
      });
      runs.push({ arm, metrics });
    } finally {
      await restore();
      await expectCssFallbackTargets(page);
    }
  }
  return runs;
}

async function expectCompositorTargets(page: Page) {
  await expect
    .poll(() =>
      page
        .locator("[data-compositor-pulse], .spinner-grid-cube")
        .evaluateAll(
          (elements) =>
            elements.length > 0 &&
            elements.every((element) =>
              element
                .getAnimations()
                .some(
                  (animation) =>
                    animation.constructor.name === "Animation" && animation.playState === "running",
                ),
            ),
        ),
    )
    .toBe(true);
}

async function installCssFallbackControl(page: Page): Promise<() => Promise<void>> {
  const style = await page.addStyleTag({
    content: `
      *,*::before,*::after{animation:none!important;transition:none!important}
      .spinner-grid-cube{animation:spinner-grid 1.3s ease-in-out infinite!important}
      .chat-input-glow-running{animation:chat-input-glow-pulse 3s ease-in-out infinite!important}
      .chat-input-glow-starting{animation:chat-input-glow-pulse 2s ease-in-out infinite!important}
    `,
  });
  await page.locator("[data-compositor-pulse], .spinner-grid-cube").evaluateAll((elements) => {
    for (const element of elements) {
      for (const animation of element.getAnimations()) {
        if (animation.constructor.name === "Animation") animation.cancel();
      }
      (element as HTMLElement).style.removeProperty("animation");
    }
  });
  await expectCssFallbackTargets(page);
  return async () => {
    await style.evaluate((element) => element.remove()).catch(() => undefined);
  };
}

async function expectCssFallbackTargets(page: Page) {
  await expect
    .poll(() =>
      page
        .locator("[data-compositor-pulse], .spinner-grid-cube")
        .evaluateAll(
          (elements) =>
            elements.length > 0 &&
            elements.every((element) =>
              element
                .getAnimations()
                .some(
                  (animation) =>
                    animation.constructor.name === "CSSAnimation" &&
                    animation.playState === "running",
                ),
            ),
        ),
    )
    .toBe(true);
}

async function expectCssFallbackGroupRunning(page: Page, selector: string) {
  await expect
    .poll(() =>
      page
        .locator(selector)
        .evaluateAll(
          (elements) =>
            elements.length > 0 &&
            elements.every((element) =>
              element
                .getAnimations()
                .some(
                  (animation) =>
                    animation.constructor.name === "CSSAnimation" &&
                    animation.playState === "running",
                ),
            ),
        ),
    )
    .toBe(true);
}

async function expectMotionPaused(page: Page, selectors: string[], all: boolean) {
  await expect
    .poll(() =>
      page.evaluate(
        ({ selectors: targetSelectors, pauseAll }) => {
          const animations = document.getAnimations().filter((animation) => {
            if (pauseAll) return true;
            const target = animation.effect?.target;
            return (
              target instanceof Element &&
              targetSelectors.some((selector) => target.matches(selector))
            );
          });
          return (
            animations.length > 0 &&
            animations.every((animation) => animation.playState !== "running")
          );
        },
        { selectors, pauseAll: all },
      ),
    )
    .toBe(true);
}

async function pauseMotion(
  page: Page,
  selectors: string[],
  all: boolean,
): Promise<() => Promise<void>> {
  await page.evaluate(
    ({ selectors: targetSelectors, pauseAll }) => {
      const state = {
        animations: document
          .getAnimations()
          .filter((animation) => {
            if (pauseAll) return true;
            const target = animation.effect?.target;
            return (
              target instanceof Element &&
              targetSelectors.some((selector) => target.matches(selector))
            );
          })
          .map((animation) => ({ animation, playState: animation.playState })),
        style: document.createElement("style"),
      };
      const cssSelector = pauseAll
        ? "*,*::before,*::after"
        : targetSelectors
            .flatMap((selector) => [selector, `${selector}::before`, `${selector}::after`])
            .join(",");
      state.style.textContent = `${cssSelector}{animation-play-state:paused!important}`;
      document.head.appendChild(state.style);
      for (const entry of state.animations) entry.animation.pause();
      const pageWindow = window as Window & {
        __kandevMotionPauseState?: typeof state;
      };
      pageWindow.__kandevMotionPauseState = state;
    },
    { selectors, pauseAll: all },
  );
  return async () => {
    await page.evaluate(() => {
      const pageWindow = window as Window & {
        __kandevMotionPauseState?: {
          animations: Array<{ animation: Animation; playState: AnimationPlayState }>;
          style: HTMLStyleElement;
        };
      };
      const state = pageWindow.__kandevMotionPauseState;
      if (!state) return;
      state.style.remove();
      for (const { animation, playState } of state.animations) {
        if (playState === "running") animation.play();
        else if (playState === "paused") animation.pause();
        else if (playState === "finished") animation.finish();
        else animation.cancel();
      }
      delete pageWindow.__kandevMotionPauseState;
    });
  };
}

async function inspectAnimationInventory(page: Page): Promise<AnimationInventory> {
  return page.evaluate(() => {
    const describe = (element: Element): string => {
      const id = element.id ? `#${element.id}` : "";
      const testId = element.getAttribute("data-testid");
      const testIdLabel = testId ? `[data-testid=${testId}]` : "";
      const classes =
        typeof element.className === "string"
          ? `.${element.className.trim().replace(/\s+/g, ".")}`
          : "";
      return `${element.tagName.toLowerCase()}${id}${testIdLabel}${classes}`;
    };
    const css: AnimationInventory["css"] = [];
    for (const element of Array.from(document.querySelectorAll<HTMLElement>("*"))) {
      for (const pseudo of [null, "::before", "::after"] as const) {
        const computed = window.getComputedStyle(element, pseudo);
        if (computed.animationName && computed.animationName !== "none") {
          css.push({ target: describe(element), pseudo, animationName: computed.animationName });
        }
      }
    }
    const webAnimations: AnimationInventory["webAnimations"] = [];
    for (const animation of document.getAnimations()) {
      const target = animation.effect?.target;
      if (!(target instanceof Element)) continue;
      webAnimations.push({
        target: describe(target),
        constructor: animation.constructor.name,
        playState: animation.playState,
        animationName:
          target instanceof HTMLElement ? window.getComputedStyle(target).animationName : null,
      });
    }
    return { css, webAnimations };
  });
}

async function captureTrace(
  page: Page,
  testInfo: TestInfo,
  label: string,
  options: TraceCaptureOptions = {},
): Promise<TraceMetrics> {
  const client = await page.context().newCDPSession(page);
  let trace = "";
  let tracingStarted = false;
  try {
    await client.send("Emulation.setScriptExecutionDisabled", {
      value: options.disableScriptExecution ?? true,
    });
    await dwell(
      1_000,
      "clock-separation",
      "the browser does not publish an event when pending application tasks are drained",
    );
    const stream = traceStream(client);
    await client.send("Tracing.start", {
      categories: [
        "devtools.timeline",
        "disabled-by-default-devtools.timeline",
        "disabled-by-default-devtools.timeline.invalidationTracking",
        "blink.user_timing",
      ].join(","),
      transferMode: "ReturnAsStream",
    });
    tracingStarted = true;
    await dwell(
      TRACE_WINDOW_MS,
      "clock-separation",
      "the fixed acceptance trace window has no completion event",
    );
    await client.send("Tracing.end");
    tracingStarted = false;
    trace = await readTraceStream(client, await stream);
  } finally {
    if (tracingStarted) await client.send("Tracing.end").catch(() => undefined);
    await client
      .send("Emulation.setScriptExecutionDisabled", { value: false })
      .catch(() => undefined);
    await client.detach().catch(() => undefined);
  }
  await testInfo.attach(`${label}.json`, {
    body: Buffer.from(trace),
    contentType: "application/json",
  });

  const events = (JSON.parse(trace) as { traceEvents: TraceEvent[] }).traceEvents;
  const metrics = traceMetrics(events);
  await testInfo.attach(`${label}-metrics.json`, {
    body: Buffer.from(JSON.stringify(metrics, null, 2)),
    contentType: "application/json",
  });
  return metrics;
}

function traceStream(client: CDPSession): Promise<string> {
  return new Promise((resolve) => {
    client.once("Tracing.tracingComplete", (event) => resolve(event.stream));
  });
}

async function readTraceStream(client: CDPSession, stream: string): Promise<string> {
  let trace = "";
  let eof = false;
  while (!eof) {
    const chunk = await client.send("IO.read", { handle: stream });
    trace += chunk.base64Encoded ? Buffer.from(chunk.data, "base64").toString("utf8") : chunk.data;
    eof = chunk.eof;
  }
  await client.send("IO.close", { handle: stream });
  return trace;
}

function traceMetrics(events: TraceEvent[]): TraceMetrics {
  return {
    updateLayoutTree: metricNamed(events, "UpdateLayoutTree"),
    layerize: metricNamed(events, "Layerize"),
    layout: metricNamed(events, "Layout"),
    paint: metricNamed(events, "Paint"),
    runTask: metricNamed(events, "RunTask"),
    functionCall: metricNamed(events, "FunctionCall"),
    targetInvalidations: events.filter(
      (event) =>
        event.name?.includes("InvalidationTracking") &&
        TARGET_PATTERN.test(JSON.stringify(event.args)),
    ).length,
    targetInvalidationsByGroup: {
      grid: countInvalidations(events, /spinner-grid-cube/),
      pulse: countInvalidations(events, /chat-input-glow-(?:running|starting)/),
    },
  };
}

function metricNamed(events: TraceEvent[], name: string): TraceMetric {
  const matchingEvents = events.filter((event) => event.name === name);
  return {
    count: matchingEvents.length,
    durationMs: matchingEvents.reduce((total, event) => total + (event.dur ?? 0), 0) / 1_000,
  };
}

function summarizeMetrics(metrics: TraceMetrics[]): TraceMetricsSummary {
  if (metrics.length === 0) throw new Error("Cannot summarize an empty trace set");
  const summarize = (read: (metric: TraceMetrics) => number): NumericSummary =>
    summarizeNumbers(metrics.map(read));
  const summarizeTraceMetric = (
    read: (metric: TraceMetrics) => TraceMetric,
  ): { count: NumericSummary; durationMs: NumericSummary } => ({
    count: summarizeNumbers(metrics.map((metric) => read(metric).count)),
    durationMs: summarizeNumbers(metrics.map((metric) => read(metric).durationMs)),
  });

  return {
    updateLayoutTree: summarizeTraceMetric((metric) => metric.updateLayoutTree),
    layerize: summarizeTraceMetric((metric) => metric.layerize),
    layout: summarizeTraceMetric((metric) => metric.layout),
    paint: summarizeTraceMetric((metric) => metric.paint),
    runTask: summarizeTraceMetric((metric) => metric.runTask),
    functionCall: summarizeTraceMetric((metric) => metric.functionCall),
    targetInvalidations: summarize((metric) => metric.targetInvalidations),
    targetInvalidationsByGroup: {
      grid: summarize((metric) => metric.targetInvalidationsByGroup.grid),
      pulse: summarize((metric) => metric.targetInvalidationsByGroup.pulse),
    },
  };
}

function summarizeNumbers(values: number[]): NumericSummary {
  if (values.length === 0) throw new Error("Cannot summarize an empty value set");
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  const median =
    sorted.length % 2 === 0 ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[middle];
  return { min: sorted[0], median, max: sorted[sorted.length - 1] };
}

function countInvalidations(events: TraceEvent[], pattern: RegExp): number {
  return events.filter(
    (event) =>
      event.name?.includes("InvalidationTracking") && pattern.test(JSON.stringify(event.args)),
  ).length;
}

function parsePositiveInteger(value: string): number {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
}
