// Routing: /t/{taskId} (task-keyed). File name starts with "mobile-" so it
// runs on the mobile-chrome Playwright project with coarse-pointer emulation.
//
// Covers the bug: xterm.js's canvas absorbs touch events, so a vertical swipe
// over the mobile terminal does nothing. The mobile passthrough wires the
// container so single-finger drags translate into `terminal.scrollLines`.
import { type Page, expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import {
  focusTerminalForTyping,
  readTerminalBuffer,
  switchToTerminalPanel,
  waitForShellReady,
} from "./mobile-terminal-helpers";

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<void> {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
  const profile = await apiClient.createAgentProfile(agents[0].id, `${title} profile`, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
  const scrollback = Array.from({ length: 200 }, (_, index) => `scrollback line ${index + 1}`).join(
    "\n",
  );
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description: scrollback,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("expected passthrough task to start a session");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForPassthroughLoad(20_000);
  await session.waitForPassthroughLoaded(20_000);
  await session.expectPassthroughHasText("scrollback line 200", 20_000);
}

async function seedShellTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
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
  await session.waitForChatIdle();
}

async function readViewportY(
  page: Page,
  rootSelector = '[data-testid="passthrough-terminal"]',
): Promise<number> {
  return page.evaluate((selector) => {
    const panels = Array.from(document.querySelectorAll(selector));
    const visiblePanels = panels.filter((panel) => panel.getClientRects().length > 0);
    const panel = visiblePanels.at(-1) ?? panels.at(-1);
    type XC = HTMLElement & { __xtermReadViewportY?: () => number };
    const container = panel?.querySelector('[data-testid="terminal-xterm-host"]') as XC | null;
    return container?.__xtermReadViewportY?.() ?? -1;
  }, rootSelector);
}

async function swipe(
  page: Page,
  direction: "down" | "up",
  steps = 5,
  rowsToScroll = 8,
  rootSelector = '[data-testid="passthrough-terminal"]',
): Promise<void> {
  const points = await page.evaluate(
    ({ direction, rootSelector, steps, rowsToScroll }) => {
      const panel = document.querySelector(rootSelector);
      const xterms = Array.from(panel?.querySelectorAll(".xterm") ?? []);
      const xtermEl = (panel?.querySelector(".xterm.focus") ?? xterms.at(-1)) as HTMLElement | null;
      if (!xtermEl) throw new Error("xterm element not found");
      const rect = xtermEl.getBoundingClientRect();
      const cx = rect.left + rect.width / 2;
      const startY = direction === "down" ? rect.top + 16 : rect.bottom - 16;
      const rowHeight = rect.height / 24;
      const requestedDy = rowHeight * rowsToScroll;
      const maxDy = Math.max(0, rect.height - 32);
      const totalDy = Math.min(requestedDy, maxDy) * (direction === "down" ? 1 : -1);
      const stepDy = totalDy / steps;

      return Array.from({ length: steps + 1 }, (_, index) => ({
        x: cx,
        y: startY + stepDy * index,
      }));
    },
    { direction, rootSelector, steps, rowsToScroll },
  );

  // CDP dispatches browser-level touch input. Unlike page.evaluate-created
  // TouchEvents, these events are trusted and exercise Chromium's real input
  // path into xterm's canvas.
  const client = await page.context().newCDPSession(page);
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ ...points[0], id: 1 }],
    });
    for (const point of points.slice(1)) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ ...point, id: 1 }],
      });
    }
    await client.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
  } finally {
    await client.detach();
  }
}

async function typeAndRun(page: Page, command: string): Promise<void> {
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

test.describe("Mobile passthrough terminal — touch scroll", () => {
  test.describe.configure({ retries: 1 });

  test("user swipes down on the terminal to scroll into scrollback, then up to return", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 820, height: 851 });
    await expect
      .poll(() => testPage.evaluate(() => matchMedia("(pointer: coarse)").matches), {
        message:
          "Playwright project must emulate coarse pointer; check the mobile-chrome project config",
      })
      .toBe(true);
    await seedTaskWithSession(testPage, apiClient, seedData, "Touch scroll");
    await testPage.getByTestId("passthrough-terminal").locator(".xterm").click();

    const bottomViewportY = await readViewportY(testPage);
    expect(bottomViewportY).toBeGreaterThan(0);
    const documentScrollY = await testPage.evaluate(() => window.scrollY);

    // Swipe down — finger drags down → reveals older lines → viewportY drops.
    await swipe(testPage, "down");

    await expect
      .poll(() => readViewportY(testPage), {
        timeout: 5_000,
        message: "Downward swipe should scroll xterm into the scrollback (viewportY decreases)",
      })
      .toBeLessThan(bottomViewportY);

    // Swipe up enough rows to return to the bottom of the buffer.
    await swipe(testPage, "up", 5, 20);

    // Match-or-beat: any late shell output that arrives between captures bumps
    // the buffer's bottom further down, so the new viewportY may legitimately
    // exceed the snapshot. xterm clamps at the buffer boundary, so overshoot
    // is impossible.
    await expect
      .poll(() => readViewportY(testPage), {
        timeout: 5_000,
        message: "Upward swipe should return viewportY to the bottom",
      })
      .toBeGreaterThanOrEqual(bottomViewportY);

    await expect.poll(() => testPage.evaluate(() => window.scrollY)).toBe(documentScrollY);
  });

  test("keeps the mobile shell terminal scrollable without moving the document", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 393, height: 851 });
    await expect
      .poll(() => testPage.evaluate(() => matchMedia("(pointer: coarse)").matches), {
        message:
          "Playwright project must emulate coarse pointer; check the mobile-chrome project config",
      })
      .toBe(true);
    await seedShellTaskWithSession(testPage, apiClient, seedData, "Shell touch scroll");
    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage);
    await focusTerminalForTyping(testPage);
    await typeAndRun(testPage, "for i in $(seq 1 200); do echo line $i; done");

    await expect
      .poll(() => readTerminalBuffer(testPage), {
        timeout: 30_000,
        message: "Waiting for the 200th shell echo to land",
      })
      .toContain("line 200");

    const shellRootSelector = '[data-testid^="mobile-terminal-slot-"][data-state="active"]';
    const bottomViewportY = await readViewportY(testPage, shellRootSelector);
    expect(bottomViewportY).toBeGreaterThan(0);
    const documentScrollY = await testPage.evaluate(() => window.scrollY);

    await swipe(testPage, "down", 5, 8, shellRootSelector);

    await expect
      .poll(() => readViewportY(testPage, shellRootSelector), {
        timeout: 5_000,
        message: "Downward shell swipe should scroll xterm into the scrollback",
      })
      .toBeLessThan(bottomViewportY);

    await swipe(testPage, "up", 5, 20, shellRootSelector);

    await expect
      .poll(() => readViewportY(testPage, shellRootSelector), {
        timeout: 5_000,
        message: "Upward shell swipe should return viewportY to the bottom",
      })
      .toBeGreaterThanOrEqual(bottomViewportY);

    await expect.poll(() => testPage.evaluate(() => window.scrollY)).toBe(documentScrollY);
  });
});
