import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { waitForFiniteAnimations } from "./animations";

export async function openTodoTask(page: Page, apiClient: ApiClient, seed: SeedData) {
  const entries = Array.from({ length: 10 }, (_, index) => ({
    content: index === 0 ? "Write tests" : `Implement step ${index}`,
    status: ["completed", "in_progress"][index] ?? "pending",
  }));
  const task = await apiClient.createTaskWithAgent(
    seed.workspaceId,
    "Todo disclosure",
    seed.agentProfileId,
    {
      description: `e2e:plan(${JSON.stringify(entries)})\ne2e:message("Checklist ready")`,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  expect(task.session_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return ["COMPLETED", "WAITING_FOR_INPUT"].includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000 },
    )
    .toBe(true);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const trigger = session.chatStatusBar().getByTestId("todo-indicator");
  await expect(trigger).toBeVisible();
  return trigger;
}

export async function expectTodoDisclosure(page: Page, input: "touch" | "mouse") {
  const content = page.getByTestId("todo-indicator-popover").filter({ visible: true });
  await expect(content).toBeVisible();
  await expect(content).toContainText("Write tests");
  await expect(content).toContainText("Implement step 1");
  await expect(content).toContainText("1/10");
  await expect(content.getByText("Write tests", { exact: true })).toHaveClass(/line-through/);
  const list = content.locator(".overflow-y-auto");
  expect(await list.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  const initialScroll = await list.evaluate((el) => el.scrollTop);
  expect(initialScroll).toBeGreaterThan(0);
  if (input === "touch") {
    const rect = await list.boundingBox();
    expect(rect).not.toBeNull();
    const x = rect!.x + rect!.width / 2;
    const y = rect!.y + rect!.height * 0.2;
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
    for (let step = 1; step <= 8; step++) {
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: y + (rect!.height * 0.6 * step) / 8 }],
      });
    }
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await cdp.detach();
  } else {
    await list.hover();
    await page.mouse.wheel(0, -1000);
  }
  await expect.poll(() => list.evaluate((el) => el.scrollTop)).toBeLessThan(initialScroll);
  const box = await content.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await waitForFiniteAnimations(content);
  return content;
}
