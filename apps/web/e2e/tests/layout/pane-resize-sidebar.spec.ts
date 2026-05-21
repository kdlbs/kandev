import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import {
  dragHorizontalSash,
  expectApproxWidth,
  getColumnSashIndex,
  getDockviewGroupWidth,
} from "../../helpers/dockview-resize";

const WIDE_VIEWPORT = { width: 1600, height: 900 };

async function openWideTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  await page.setViewportSize(WIDE_VIEWPORT);
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
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForDockviewReady();
  return session;
}

test.describe("Sidebar resize — viewport-proportional cap", () => {
  test("resizes past the old 350px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar past cap");
    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    await dragHorizontalSash(testPage, sashIdx, +250);
    const width = await getDockviewGroupWidth(testPage, "sidebar");
    expect(width).toBeGreaterThan(450);
  });

  test("respects the viewport-proportional cap on narrower screens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1200, height: 800 });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar cap narrow",
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
    await session.waitForDockviewReady();

    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    await dragHorizontalSash(testPage, sashIdx, +2000);
    const width = await getDockviewGroupWidth(testPage, "sidebar");
    expect(width).toBeLessThanOrEqual(Math.max(800, Math.round(1200 * 0.7)) + 10);
  });

  test("user width survives reload", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar reload");
    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    await dragHorizontalSash(testPage, sashIdx, +180);
    const before = await getDockviewGroupWidth(testPage, "sidebar");

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 10);
  });

  test("user width propagates to brand-new task envs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar propagate A");
    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    await dragHorizontalSash(testPage, sashIdx, +200);
    const widthA = await getDockviewGroupWidth(testPage, "sidebar");
    void session;

    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar propagate B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${taskB.id}`);
    const sessionB = new SessionPage(testPage);
    await sessionB.waitForLoad();
    await sessionB.waitForDockviewReady();

    const widthB = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(widthB, widthA, 20);
  });

  test("toggle sidebar off+on preserves the last user width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar toggle");
    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    await dragHorizontalSash(testPage, sashIdx, +160);
    const before = await getDockviewGroupWidth(testPage, "sidebar");

    // TOGGLE_SIDEBAR shortcut (Ctrl/Cmd+B). Move focus out of any input first.
    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(200);
    await testPage.keyboard.press(`${mod}+b`);
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 20);
  });
});
