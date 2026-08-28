import type { CDPSession, Page, TestInfo } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

const TRACE_WINDOW_MS = 8_340;
const TARGET_PATTERN = /spinner-grid-cube|chat-input-glow-(?:running|starting)/;

type TraceEvent = {
  name?: string;
  args?: unknown;
};

type TraceMetrics = {
  updateLayoutTree: number;
  layerize: number;
  layout: number;
  paint: number;
  targetInvalidations: number;
};

test.skip(
  process.env.KANDEV_E2E_ANIMATION_TRACE !== "1",
  "Run explicitly to capture the 8.34-second animation performance control.",
);
test.describe.configure({ retries: 1 });

test("attributes steady animation work for compositor and CSS fallback paths", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
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
  await session.sendMessage("/slow 30s");

  const glow = session.activeChat().getByTestId("chat-input-glow");
  await expect(glow).toBeVisible();
  const dialog = await openQuickChatWithAgent(testPage);
  await sendQuickChatMessage(dialog, testPage, "/slow 30s");
  const cubes = dialog.locator(".spinner-grid-cube");
  await expect(cubes).toHaveCount(18);
  await expectCompositorTargets(testPage);

  const disableCssMotion = await testPage.addStyleTag({
    content: "*,*::before,*::after{animation:none!important;transition:none!important}",
  });
  await dwell(1_000, "clock-separation", "exclude one-time effect setup from the trace window");
  const compositor = await captureTrace(testPage, testInfo, "compositor-motion");
  expect(compositor.targetInvalidations).toBe(0);

  await disableCssMotion.evaluate((element) => element.remove());
  await installCssFallbackControl(testPage);
  await dwell(
    1_000,
    "clock-separation",
    "exclude fallback style installation from the trace window",
  );
  const cssFallback = await captureTrace(testPage, testInfo, "css-fallback-motion");
  console.info(`[animation-performance-trace] ${JSON.stringify({ compositor, cssFallback })}`);
  expect(cssFallback.updateLayoutTree).toBeGreaterThan(0);
  expect(cssFallback.layerize).toBeGreaterThan(0);
});

async function expectCompositorTargets(page: Page) {
  await expect
    .poll(() =>
      page
        .locator("[data-compositor-pulse], .spinner-grid-cube")
        .evaluateAll((elements) =>
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

async function installCssFallbackControl(page: Page) {
  await page.addStyleTag({
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
  await expect
    .poll(() =>
      page
        .locator("[data-compositor-pulse], .spinner-grid-cube")
        .evaluateAll((elements) =>
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

async function captureTrace(page: Page, testInfo: TestInfo, label: string): Promise<TraceMetrics> {
  const client = await page.context().newCDPSession(page);
  let trace = "";
  let tracingStarted = false;
  try {
    await client.send("Emulation.setScriptExecutionDisabled", { value: true });
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
    updateLayoutTree: countNamed(events, "UpdateLayoutTree"),
    layerize: countNamed(events, "Layerize"),
    layout: countNamed(events, "Layout"),
    paint: countNamed(events, "Paint"),
    targetInvalidations: events.filter(
      (event) =>
        event.name?.includes("InvalidationTracking") &&
        TARGET_PATTERN.test(JSON.stringify(event.args)),
    ).length,
  };
}

function countNamed(events: TraceEvent[], name: string): number {
  return events.filter((event) => event.name === name).length;
}
