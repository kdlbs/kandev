import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * The sidebar header (h-10) must line up exactly with the surface top bar so the
 * bottom border reads as one continuous line across the sidebar/content seam.
 * On a task page that surface bar is the TaskTopBar — which previously had no
 * explicit height and so sat a couple of px off from the 40px sidebar header.
 */
async function bottomOf(page: Page, testId: string): Promise<number> {
  const box = await page.getByTestId(testId).boundingBox();
  if (!box) throw new Error(`no bounding box for ${testId}`);
  return box.y + box.height;
}

async function heightOf(page: Page, testId: string): Promise<number> {
  const box = await page.getByTestId(testId).boundingBox();
  if (!box) throw new Error(`no bounding box for ${testId}`);
  return box.height;
}

async function enableTopbarMetrics(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    system_metrics_display: { show_in_topbar: true },
  });
}

test.describe("Sidebar header / top bar alignment", () => {
  test.describe.configure({ retries: 1 });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: false },
    });
  });

  test("task top bar bottom edge aligns with the sidebar header", async ({
    testPage,
    apiClient,
    seedData,
  }: {
    testPage: Page;
    apiClient: ApiClient;
    seedData: SeedData;
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Alignment Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
    const session = new SessionPage(testPage);
    await session.waitForDockviewReady(30_000);

    const topbar = testPage.getByTestId("task-topbar");
    await expect(topbar).toBeVisible({ timeout: 30_000 });

    // Read the named header directly so layout wrappers can change without
    // weakening the alignment check (both top bars are 40px high).
    await expect(testPage.getByTestId("app-sidebar-header")).toBeVisible();
    const headerBottom = await bottomOf(testPage, "app-sidebar-header");
    const topbarBottom = await bottomOf(testPage, "task-topbar");

    // Allow 1px for sub-pixel rounding of the shared border line.
    expect(Math.abs(headerBottom - topbarBottom)).toBeLessThanOrEqual(1);
  });

  test("task metrics match the height of the task action controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await enableTopbarMetrics(apiClient);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Metrics Alignment Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(testPage.getByTestId("topbar-metrics")).toBeVisible();
    await expect(testPage.getByTestId("layout-preset-trigger")).toBeVisible();
    expect(await heightOf(testPage, "topbar-metrics")).toBe(
      await heightOf(testPage, "layout-preset-trigger"),
    );
  });

  test("Kanban metrics match the height of the Kanban action controls", async ({
    testPage,
    apiClient,
  }) => {
    await enableTopbarMetrics(apiClient);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(testPage.getByTestId("topbar-metrics")).toBeVisible();
    await expect(testPage.getByTestId("view-toggle-kanban")).toBeVisible();
    expect(await heightOf(testPage, "topbar-metrics")).toBe(
      await heightOf(testPage, "view-toggle-kanban"),
    );
  });
});
