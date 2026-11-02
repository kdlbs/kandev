import type { DockviewApi } from "dockview-react";
import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";

async function apiPanelIds(page: import("@playwright/test").Page) {
  return page.evaluate(() => {
    const api = (window as Window & { __dockviewApi__?: DockviewApi | null }).__dockviewApi__;
    return api ? api.panels.map((panel) => panel.id).sort() : null;
  });
}

test("releases the departed Dockview API without losing drafts on return or responsive changes", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const task = await api.createTask(seed.workspaceId, "Dockview owner release", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const { session_id: sessionId } = await api.seedTaskSession(task.id, {
    sessionId: `owner-release-${task.id}`,
    state: "WAITING_FOR_INPUT",
    agentProfileId: seed.agentProfileId,
  });
  await api.seedSessionMessage(sessionId, {
    type: "message",
    content: "Dockview owner release history",
    authorType: "agent",
  });
  await page.goto(`/t/${task.id}?sessionId=${sessionId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.activeChat()).toContainText("Dockview owner release history");
  await expect.poll(() => apiPanelIds(page)).not.toBeNull();
  const panels = await apiPanelIds(page);
  expect(panels!.length).toBeGreaterThan(0);
  const origin = await page.evaluate(() => performance.timeOrigin);
  const draft = "Keep this draft after leaving the task and changing the viewport";
  await session.activeChat().locator(".tiptap").fill(draft);
  for (let visit = 0; visit < 3; visit++) {
    await page.getByTestId("sidebar-settings-gear").click();
    await expect(page).toHaveURL(/\/settings(?:\/|$)/);
    await expect(page.getByTestId("dockview-task-layout")).toHaveCount(0);
    await expect.poll(() => apiPanelIds(page)).toBeNull();
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(origin);
    await page.goBack();
    await session.waitForLoad();
    await expect.poll(() => apiPanelIds(page)).toEqual(panels);
    await expect(session.activeChat().locator(".tiptap")).toHaveText(draft);
  }
  const viewport = page.viewportSize()!;
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByTestId("mobile-task-layout")).toBeVisible();
  await expect.poll(() => apiPanelIds(page)).toBeNull();
  await expect(session.activeChat().locator(".tiptap")).toHaveText(draft);
  if (prCapture.capturing) {
    await waitForFiniteAnimations(page.getByTestId("mobile-task-layout"));
    await prCapture.screenshot("phone", {
      caption: "Draft preserved after desktop layout release",
    });
  }
  await page.setViewportSize(viewport);
  await expect(page.getByTestId("dockview-task-layout")).toBeVisible();
  await expect.poll(() => apiPanelIds(page)).toEqual(panels);
  await expect(session.activeChat().locator(".tiptap")).toHaveText(draft);
  if (prCapture.capturing) {
    await waitForFiniteAnimations(page.getByTestId("dockview-task-layout"));
    await prCapture.screenshot("desktop", { caption: "Draft and panels restored after return" });
  }
});
