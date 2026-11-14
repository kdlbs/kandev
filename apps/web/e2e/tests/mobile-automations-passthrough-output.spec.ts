import type { Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import { dwell, waitForHttp } from "../helpers/causal-waits";
import {
  seedAutomationPassthroughOutput,
  PASSTHROUGH_GUIDANCE_DESCRIPTION,
  PASSTHROUGH_GUIDANCE_TITLE,
} from "../helpers/automation-passthrough-output";

function observeReadOnlyRunRequests(page: Page) {
  const terminalSockets: string[] = [];
  const sessionStartRequests: string[] = [];
  page.on("websocket", (socket) => {
    const socketUrl = new URL(socket.url());
    if (socketUrl.pathname.includes("/terminal/session/")) terminalSockets.push(socket.url());
    if (!socketUrl.pathname.endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      try {
        const frame = JSON.parse(payload.toString()) as { action?: string };
        if (["session.launch", "session.resume", "session.restart"].includes(frame.action ?? "")) {
          sessionStartRequests.push(frame.action ?? "");
        }
      } catch {
        // Binary gateway frames do not contain a lifecycle request.
      }
    });
  });
  return { terminalSockets, sessionStartRequests };
}

function waitForSessionHydration(page: Page, sessionId: string) {
  return waitForHttp(page, "GET", new RegExp(`/api/v1/task-sessions/${sessionId}$`));
}

async function expectPersistedMode(
  response: Awaited<ReturnType<typeof waitForHttp>>,
  mode: boolean,
) {
  const payload = (await response.json()) as { session?: { is_passthrough?: boolean } };
  expect(payload.session?.is_passthrough).toBe(mode);
}

async function expectRunContent(
  page: Page,
  run: { id: string; status: string; prompt: string; error: string },
) {
  const transcript = page.getByTestId("run-transcript");
  await expect(transcript).toBeVisible();
  await expect(transcript).toContainText(run.prompt);
  await expect(transcript).toContainText(run.error);
  await expect(page.getByTestId("automation-activity")).toContainText(
    run.status === "task_created" ? "Running" : "Succeeded",
  );
  await expect(transcript.getByTestId("chat-input-editor")).toBeVisible();
}

test("keeps passthrough guidance usable while switching runs on a phone", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { automation, runs } = await seedAutomationPassthroughOutput(
    apiClient,
    seedData,
    "Passthrough output phone",
  );
  const observations = observeReadOnlyRunRequests(testPage);
  let hydrated = waitForSessionHydration(testPage, runs.parkedPassthrough.sessionId);
  await testPage.goto(`/automations/${automation.id}?run=${runs.parkedPassthrough.id}`);
  await expectPersistedMode(await hydrated, true);

  const guidance = testPage.getByTestId("passthrough-capture-guidance");
  await expect(guidance).toContainText(PASSTHROUGH_GUIDANCE_TITLE);
  await expect(guidance).toContainText(PASSTHROUGH_GUIDANCE_DESCRIPTION);
  await expectRunContent(testPage, runs.parkedPassthrough);

  hydrated = waitForSessionHydration(testPage, runs.parkedPassthrough.sessionId);
  await testPage.reload();
  await expectPersistedMode(await hydrated, true);
  await expect(testPage.getByTestId("passthrough-capture-guidance")).toContainText(
    PASSTHROUGH_GUIDANCE_TITLE,
  );
  await expectRunContent(testPage, runs.parkedPassthrough);

  const description = guidance.locator("p").nth(1);
  const guidanceLayout = await description.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    return {
      height: bounds.height,
      lineHeight: Number.parseFloat(getComputedStyle(element).lineHeight),
      right: bounds.right,
      viewportWidth: window.innerWidth,
    };
  });
  expect(guidanceLayout.height).toBeGreaterThan(guidanceLayout.lineHeight);
  expect(guidanceLayout.right).toBeLessThanOrEqual(guidanceLayout.viewportWidth);

  const trigger = testPage.getByTestId("runs-drawer-trigger");
  await trigger.tap();
  const drawer = testPage.getByTestId("runs-drawer");
  await expect(drawer).toBeVisible();
  hydrated = waitForSessionHydration(testPage, runs.structured.sessionId);
  await drawer.getByTestId(`run-row-${runs.structured.id}`).tap();
  await expect(drawer).toHaveCount(0);
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expectPersistedMode(await hydrated, false);
  await expect(testPage.getByTestId("passthrough-capture-guidance")).toHaveCount(0);
  await expectRunContent(testPage, runs.structured);

  await trigger.tap();
  await expect(drawer).toBeVisible();
  hydrated = waitForSessionHydration(testPage, runs.runningPassthrough.sessionId);
  await drawer.getByTestId(`run-row-${runs.runningPassthrough.id}`).tap();
  await expect(drawer).toHaveCount(0);
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expectPersistedMode(await hydrated, true);
  await expect(testPage.getByTestId("passthrough-capture-guidance")).toContainText(
    PASSTHROUGH_GUIDANCE_DESCRIPTION,
  );
  await expectRunContent(testPage, runs.runningPassthrough);
  const composer = testPage.getByTestId("chat-input-area");
  const composerBounds = await composer.boundingBox();
  const viewport = testPage.viewportSize();
  expect(composerBounds, "reply composer should have a visible box on the phone").not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(composerBounds!.x).toBeGreaterThanOrEqual(0);
  expect(composerBounds!.x + composerBounds!.width).toBeLessThanOrEqual(viewport!.width);
  expect(composerBounds!.y + composerBounds!.height).toBeLessThanOrEqual(viewport!.height);
  const pageWidth = await testPage.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
  await prCapture.screenshot("passthrough-run-guidance-phone", {
    caption:
      "Phone run reader wraps passthrough capture guidance above its transcript and composer.",
  });

  await dwell(
    testPage,
    300,
    "negative-assertion",
    "passive run reading must not connect a terminal or start or resume its session",
  );
  expect(observations.terminalSockets).toEqual([]);
  expect(observations.sessionStartRequests).toEqual([]);
});
