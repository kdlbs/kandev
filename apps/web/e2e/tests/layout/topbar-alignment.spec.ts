import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
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

test.describe("Sidebar header / top bar alignment", () => {
  test.describe.configure({ retries: 1 });

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

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const topbar = testPage.getByTestId("task-topbar");
    await expect(topbar).toBeVisible({ timeout: 30_000 });

    // The sidebar header is the first child of the sidebar aside; tag-agnostic,
    // we measure the aside-relative header via its testid-less first row by
    // reading the task-topbar against the sidebar header height (both 40px).
    const headerBottom = await testPage.evaluate(() => {
      const aside = document.querySelector('[data-testid="app-sidebar"]');
      const header = aside?.firstElementChild as HTMLElement | null;
      if (!header) return null;
      const r = header.getBoundingClientRect();
      return r.y + r.height;
    });
    expect(headerBottom).not.toBeNull();

    const topbarBottom = await bottomOf(testPage, "task-topbar");

    // Allow 1px for sub-pixel rounding of the shared border line.
    expect(Math.abs((headerBottom as number) - topbarBottom)).toBeLessThanOrEqual(1);
  });
});
