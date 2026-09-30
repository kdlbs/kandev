// AC-COORDINATOR-WAKE-005, -006 and the autonomy settings section: the
// autonomy strip, the held Needs you item, the Autonomy settings section and
// the "Woken by" transcript entry, on a desktop and a 390px viewport.
//
// The autonomy and run reads are stubbed at the network edge. Their wire
// contract is proven against the real backend by the Go route tests; what this
// file proves is the browser rendering of that contract, including states
// (a held reason with a failing stop, a run with denied permissions) that a
// mock agent cannot produce on demand.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  linkToCoordinatorAutonomySettings,
  linkToCoordinatorNeedsYou,
  linkToCoordinatorQueue,
} from "../../../lib/coordinator/links";
import { heldAutonomyBody, runBody, stubAutonomyRead } from "./autonomy-fixture";

const PHONE = { width: 390, height: 844 };
const MIN_TOUCH_TARGET_PX = 44;
const TURN_ID = "turn-e2e-1";
const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;

const LINE_TOLERANCE_PX = 6;

/** The number of visual text lines, clustering the text's client rects by vertical centre. */
async function lineCount(locator: Locator): Promise<number> {
  return locator.evaluate((el, tolerance) => {
    const centres: number[] = [];
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (!node.textContent?.trim()) continue;
      const range = document.createRange();
      range.selectNodeContents(node);
      for (const rect of Array.from(range.getClientRects())) {
        centres.push(rect.top + rect.height / 2);
      }
    }
    const lines: number[] = [];
    for (const centre of centres.sort((x, y) => x - y)) {
      if (lines.length === 0 || centre - lines[lines.length - 1] > tolerance) lines.push(centre);
    }
    return lines.length;
  }, LINE_TOLERANCE_PX);
}

async function makeCoordinator(
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string },
  name: string,
) {
  return apiClient.createCoordinator(seedData.workspaceId, {
    name,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });
}

async function openCopilot(page: Page): Promise<{ popover: Locator; sessionId: string }> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  const response = await conversationOpened;
  const body = (await response.json()) as { session_id: string };
  const popover = page.getByTestId("coordinator-copilot-popover");
  await expect(popover).toBeVisible();
  return { popover, sessionId: body.session_id };
}

function tagFrame(line: string): string {
  try {
    const frame = JSON.parse(line) as {
      action?: string;
      payload?: { messages?: Array<Record<string, unknown>> };
    };
    if (frame.action !== "message.list") return line;
    for (const message of frame.payload?.messages ?? []) {
      if (message.author_type === "user") {
        message.metadata = { ...(message.metadata as object), coordinator_wake_turn_id: TURN_ID };
      }
    }
    return JSON.stringify(frame);
  } catch {
    return line;
  }
}

async function tagUserMessagesOverWs(page: Page): Promise<void> {
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => server.send(message));
    server.onMessage((message) =>
      socket.send(
        typeof message === "string" ? message.split("\n").map(tagFrame).join("\n") : message,
      ),
    );
  });
}

/** Marks the conversation's user messages as an unattended wake and stubs the run read. */
async function stubWakeTranscript(page: Page, coordinatorId: string): Promise<void> {
  await tagUserMessagesOverWs(page);
  await page.route(new RegExp(`/coordinators/${coordinatorId}/runs/${TURN_ID}$`), (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      json: runBody(TURN_ID, coordinatorId),
    }),
  );
}

async function sendOneMessage(popover: Locator): Promise<void> {
  const editor = popover.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill("/e2e:simple-message");
  await editor.press(`${modifier}+Enter`);
  await expect(
    popover.getByText("simple mock response for e2e testing", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
}

test.describe("Coordinator autonomy on desktop", () => {
  test("the strip, the held item and the settings section agree on a containment hold", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await makeCoordinator(apiClient, seedData, "Autonomy Coordinator");
    await stubAutonomyRead(testPage, coordinator.id);

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const strip = testPage.getByTestId("autonomy-strip");
    await expect(strip).toBeVisible();
    await expect(strip.getByTestId("autonomy-strip-state")).toHaveAttribute("data-state", "held");
    await expect(strip.getByTestId("autonomy-strip-state")).toContainText("Autonomy: Held");
    await expect(strip.getByTestId("autonomy-strip-pending")).toContainText("5 pending");
    await expect(strip.getByTestId("autonomy-spend-pill")).toHaveAttribute("data-pill", "over");
    await expect(strip.getByTestId("autonomy-stop-warning")).toBeVisible();
    await expect(strip.getByTestId("autonomy-stop")).toBeVisible();
    await expect(strip.getByText("Open settings")).toHaveCount(0);

    const item = testPage.getByTestId(`needs-you-item-autonomy:${coordinator.id}`);
    await expect(item).toBeVisible();
    await expect(item.getByTestId("autonomy-item-why")).toContainText("5 events");
    await item.getByTestId("autonomy-item-open-settings").click();

    await expect(testPage).toHaveURL(/section=autonomy/);
    const section = testPage.getByTestId("autonomy-section");
    await expect(section).toBeVisible();
    await expect(section.getByTestId("autonomy-spend-pill")).toHaveAttribute("data-pill", "over");
    const credential = section.getByTestId("containment-no_kandev_credential");
    await expect(credential).toHaveAttribute("data-met", "false");
    await expect(credential.getByTestId("containment-token")).toHaveText("KANDEV_API_KEY");
    await expect(section.getByTestId("containment-auth_enabled")).toHaveAttribute(
      "data-met",
      "true",
    );
  });

  test("the strip also renders on Queue and a failed read shows one Try again line", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await makeCoordinator(apiClient, seedData, "Autonomy Queue Coordinator");
    let failing = true;
    await testPage.route(new RegExp(`/coordinators/${coordinator.id}/autonomy$`), (route) =>
      failing
        ? route.fulfill({ status: 500, json: { error: "read_error" } })
        : route.fulfill({ status: 200, json: heldAutonomyBody() }),
    );

    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId("autonomy-strip-unavailable")).toBeVisible();
    await expect(testPage.getByTestId("autonomy-strip-state")).toHaveCount(0);

    failing = false;
    await testPage.getByTestId("autonomy-strip-retry").click();
    await expect(testPage.getByTestId("autonomy-strip-state")).toHaveAttribute(
      "data-state",
      "held",
    );
  });

  test("the settings section saves the toggle and ceiling as one PATCH", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await makeCoordinator(apiClient, seedData, "Autonomy Settings Coordinator");
    await stubAutonomyRead(testPage, coordinator.id);

    await testPage.goto(linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId("autonomy-section")).toBeVisible();

    await testPage.getByTestId("autonomy-toggle").click();
    await expect(testPage.getByTestId("autonomy-ceiling-hint")).toHaveAttribute(
      "data-hint",
      "required",
    );
    await testPage.getByTestId("autonomy-ceiling").fill("12.5");
    await expect(testPage.getByTestId("autonomy-ceiling-hint")).toHaveCount(0);

    const patched = waitForHttp(testPage, "PATCH", /\/coordinators\/[^/]+$/);
    await testPage.getByRole("button", { name: "Save" }).click();
    const response = await patched;
    expect(response.request().postDataJSON()).toEqual({
      autonomy_enabled: true,
      cost_ceiling_usd: "12.50",
    });
    const saved = (await response.json()) as {
      autonomy_enabled?: boolean;
      cost_ceiling_usd?: string;
    };
    expect(saved.autonomy_enabled).toBe(true);
    expect(saved.cost_ceiling_usd).toBe("12.50");
  });

  test("the copilot renders an unattended message as a collapsed Woken by entry from the run read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const coordinator = await makeCoordinator(apiClient, seedData, "Woken By Coordinator");
    await stubAutonomyRead(testPage, coordinator.id);
    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const { popover } = await openCopilot(testPage);
    await sendOneMessage(popover);

    await stubWakeTranscript(testPage, coordinator.id);
    await testPage.reload();
    const reopened = await openCopilot(testPage);

    const entry = reopened.popover.getByTestId("woken-by-entry");
    await expect(entry).toHaveAttribute("data-state", "loaded");
    await expect(entry.getByTestId("woken-by-header")).toHaveText("Woken by 3 events");
    await expect(entry.getByTestId("woken-by-denied")).toHaveText("1 permission denied");
    const toggle = entry.getByTestId("woken-by-toggle");
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(entry.getByTestId("woken-by-row")).toHaveCount(0);
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(entry.getByTestId("woken-by-row")).toHaveCount(3);
    await expect(entry.getByTestId("woken-by-row").first()).toContainText("KAN-418");
  });
});

test.describe("Coordinator autonomy at 390px", () => {
  test.use({ hasTouch: true });

  test("the strip wraps to at most three lines with no horizontal scroll and touch-sized controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize(PHONE);
    const coordinator = await makeCoordinator(apiClient, seedData, "Autonomy Phone Coordinator");
    await stubAutonomyRead(testPage, coordinator.id);

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const line = testPage.getByTestId("autonomy-strip-line");
    await expect(line).toBeVisible();
    expect(await lineCount(line)).toBeLessThanOrEqual(3);
    const scroll = await testPage.evaluate(() => document.documentElement.scrollWidth);
    expect(scroll).toBeLessThanOrEqual(PHONE.width);

    const stop = await testPage.getByTestId("autonomy-stop").boundingBox();
    expect(stop!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    const openSettings = await testPage
      .getByTestId(`needs-you-item-autonomy:${coordinator.id}`)
      .getByTestId("autonomy-item-open-settings")
      .boundingBox();
    expect(openSettings!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    await expect(testPage.getByTestId("autonomy-strip").getByText("Open settings")).toHaveCount(0);
  });

  test("the Woken by list toggle stays inside its message bubble", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize(PHONE);
    const coordinator = await makeCoordinator(apiClient, seedData, "Woken By Phone Coordinator");
    await stubAutonomyRead(testPage, coordinator.id);
    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const { popover } = await openCopilot(testPage);
    const editor = popover.getByTestId("chat-input-editor");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
    await editor.pressSequentially("/e2e:simple-message", { timeout: 15_000 });
    await popover.getByTestId("submit-message-button").tap();
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    await stubWakeTranscript(testPage, coordinator.id);
    await testPage.reload();
    const reopened = await openCopilot(testPage);
    const entry = reopened.popover.getByTestId("woken-by-entry");
    await expect(entry).toHaveAttribute("data-state", "loaded");
    await entry.getByTestId("woken-by-toggle").tap();
    await expect(entry.getByTestId("woken-by-row")).toHaveCount(3);

    const bubble = await entry.boundingBox();
    const toggle = await entry.getByTestId("woken-by-toggle").boundingBox();
    expect(toggle!.x).toBeGreaterThanOrEqual(bubble!.x);
    expect(toggle!.x + toggle!.width).toBeLessThanOrEqual(bubble!.x + bubble!.width + 0.5);
    expect(toggle!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(bubble!.x + bubble!.width).toBeLessThanOrEqual(PHONE.width);
    const scroll = await testPage.evaluate(() => document.documentElement.scrollWidth);
    expect(scroll).toBeLessThanOrEqual(PHONE.width);
  });
});
