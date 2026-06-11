// Routing: /t/{taskId} (task-keyed). File name starts with "mobile-" so it
// runs on the mobile-chrome Playwright project (Pixel 5 emulation).
//
// Covers the bug: xterm.js's canvas absorbs touch events, so a vertical swipe
// over the mobile terminal does nothing. The mobile passthrough wires the
// container so single-finger drags translate into `terminal.scrollLines`.
import { type Page, expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
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
  return session;
}

async function tapTerminalTab(testPage: Page): Promise<void> {
  await testPage.getByRole("button", { name: "Terminal" }).tap();
}

async function switchToTerminalPanel(testPage: Page): Promise<void> {
  // Confirm the panel actually mounted rather than firing a single tap. On
  // mobile the bottom-nav button can be tapped before hydration wires its
  // handler; a lost tap leaves the terminal panel unmounted, which would later
  // strand waitForShellReady polling an element that never appears. Re-tap once
  // if the first tap didn't take.
  const panel = testPage.getByTestId("terminal-panel");
  await tapTerminalTab(testPage);
  if (!(await panel.isVisible())) {
    await tapTerminalTab(testPage);
  }
  await expect(panel).toBeVisible({ timeout: 10_000 });
}

async function waitForShellReady(testPage: Page, timeout = 45_000): Promise<void> {
  // The shell WS connect can be missed under CI load (the auto-create guard
  // only retries on a WS reconnect). If the panel falls out of view we re-tap
  // it, which forces a remount and kicks the reconnect loop — so we don't
  // blindly wait out the whole budget on a dead connection.
  const panel = testPage.getByTestId("terminal-panel");
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if ((await readTerminalBuffer(testPage)).length > 0) return;
    if (!(await panel.isVisible())) {
      await switchToTerminalPanel(testPage);
    }
    await testPage.waitForTimeout(1_000);
  }
  expect(
    (await readTerminalBuffer(testPage)).length,
    "Waiting for mobile terminal shell to connect",
  ).toBeGreaterThan(0);
}

async function readTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    const xtermEl = panel?.querySelector(".xterm");
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

async function readViewportY(page: Page): Promise<number> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    const xtermEl = panel?.querySelector(".xterm");
    type XC = HTMLElement & { __xtermReadViewportY?: () => number };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadViewportY?.() ?? -1;
  });
}

async function focusTerminalForTyping(session: SessionPage): Promise<void> {
  await session.terminal.locator(".xterm").click();
}

/**
 * Dispatch a synthetic single-finger swipe on the xterm container. We bypass
 * Playwright's touchscreen because the gesture coordinates need to be relative
 * to the .xterm element's bounding box, and we want guaranteed delivery
 * regardless of overlay z-index.
 */
async function swipe(
  page: Page,
  direction: "down" | "up",
  steps = 5,
  rowsToScroll = 8,
): Promise<void> {
  await page.evaluate(
    ({ direction, steps, rowsToScroll }) => {
      const panel = document.querySelector('[data-testid="terminal-panel"]');
      const xtermEl = panel?.querySelector(".xterm") as HTMLElement | null;
      if (!xtermEl) throw new Error("xterm element not found");
      const rect = xtermEl.getBoundingClientRect();
      const cx = rect.left + rect.width / 2;
      const startY = direction === "down" ? rect.top + 16 : rect.bottom - 16;
      const rowHeight = rect.height / 24;
      const totalDy = rowHeight * rowsToScroll * (direction === "down" ? 1 : -1);
      const stepDy = totalDy / steps;

      const makeTouch = (clientX: number, clientY: number) =>
        ({ clientX, clientY, identifier: 1 }) as unknown as Touch;
      const fire = (type: string, clientY: number) => {
        const event = new Event(type, { bubbles: true, cancelable: true }) as TouchEvent;
        const touches = [makeTouch(cx, clientY)];
        Object.defineProperty(event, "touches", { value: touches, configurable: true });
        Object.defineProperty(event, "changedTouches", { value: touches, configurable: true });
        xtermEl.dispatchEvent(event);
      };

      fire("touchstart", startY);
      for (let i = 1; i <= steps; i++) {
        fire("touchmove", startY + stepDy * i);
      }
      fire("touchend", startY + totalDy);
    },
    { direction, steps, rowsToScroll },
  );
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
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Touch scroll");
    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage);
    await focusTerminalForTyping(session);

    // Produce enough output to populate the scrollback past one viewport.
    await typeAndRun(testPage, "for i in $(seq 1 200); do echo line $i; done");

    await expect
      .poll(() => readTerminalBuffer(testPage), {
        timeout: 30_000,
        message: "Waiting for the 200th echo to land",
      })
      .toContain("line 200");

    const bottomViewportY = await readViewportY(testPage);
    expect(bottomViewportY).toBeGreaterThan(0);

    // Swipe down — finger drags down → reveals older lines → viewportY drops.
    await swipe(testPage, "down");

    await expect
      .poll(() => readViewportY(testPage), {
        timeout: 5_000,
        message: "Downward swipe should scroll xterm into the scrollback (viewportY decreases)",
      })
      .toBeLessThan(bottomViewportY);

    // Swipe up enough rows to return to the bottom of the buffer.
    await swipe(testPage, "up", 5, 50);

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
  });
});
