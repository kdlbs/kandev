import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import {
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

async function createTaskAt(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  viewport: { width: number; height: number },
  title: string,
): Promise<SessionPage> {
  await page.setViewportSize(viewport);
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
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 480);
    expect(actual).toBeGreaterThan(420);
  });

  test("respects the viewport-proportional cap on narrower screens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await createTaskAt(
      testPage,
      apiClient,
      seedData,
      { width: 1200, height: 800 },
      "Sidebar cap narrow",
    );
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 5000);
    expect(actual).toBeLessThanOrEqual(Math.max(350, Math.round(1200 * 0.3)) + 10);
  });

  test("user width survives reload", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar reload");
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 440);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 12);
  });

  test("user width propagates to brand-new task envs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar propagate A");
    const widthA = await resizeColumnViaSplitview(testPage, "sidebar", 450);
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
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 430);

    // TOGGLE_SIDEBAR shortcut (Ctrl/Cmd+B). Move focus out of any input first.
    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(250);
    await testPage.keyboard.press(`${mod}+b`);
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 20);
  });
});
