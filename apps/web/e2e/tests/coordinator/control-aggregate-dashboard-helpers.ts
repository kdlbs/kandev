import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

const STALLED_TITLE = "Aggregate stalled task";
const BLOCKED_TITLE = "Aggregate blocked task";
const REPEATED_ACTION = "aggregate_e2e_repeat";

async function assertNoPageOverflow(page: Page) {
  const width = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
}

/**
 * The integrated control overview and history read the Office multi-workspace
 * aggregate: a workspace with in-progress work and no working agent shows as
 * stalled, blocked tasks raise an attention row, and repeated activity folds.
 */
export function controlAggregateDashboardCases(mobile: boolean) {
  test("integrated control shows the Office aggregate stall, attention and comparison", async ({
    testPage,
    backend,
    apiClient,
    officeSeed,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_INTEGRATED_CONTROL: "true" });
    try {
      const { workspaces } = await apiClient.listWorkspaces();
      const workspaceName = workspaces.find((w) => w.id === officeSeed.workspaceId)?.name;
      expect(workspaceName).toBeTruthy();
      const stalled = await apiClient.seedTask(officeSeed.workspaceId, STALLED_TITLE, {
        workflow_id: officeSeed.workflowId,
        state: "IN_PROGRESS",
      });
      await apiClient.seedTask(officeSeed.workspaceId, BLOCKED_TITLE, {
        workflow_id: officeSeed.workflowId,
        state: "BLOCKED",
      });
      for (let index = 0; index < 3; index += 1) {
        await apiClient.seedActivity({
          workspaceId: officeSeed.workspaceId,
          actorType: "agent",
          actorId: officeSeed.agentId,
          action: REPEATED_ACTION,
          targetType: "task",
          targetId: stalled.task_id,
        });
      }

      await testPage.goto("/control/overview");
      const band = testPage.getByTestId("control-stall-band");
      await expect(band).toBeVisible();
      await expect(band).not.toHaveAttribute("data-stalled", "0");

      const row = testPage
        .getByTestId("control-compare-row")
        .filter({ has: testPage.getByRole("link", { name: workspaceName!, exact: true }) });
      await expect(row).toHaveAttribute("data-status", "stalled");
      await expect(row.getByRole("link", { name: workspaceName!, exact: true })).toHaveAttribute(
        "href",
        `/office?workspaceId=${officeSeed.workspaceId}`,
      );

      const attention = testPage.getByTestId("control-attention-item").filter({
        hasText: workspaceName!,
      });
      await expect(attention.and(testPage.locator('[data-kind="stalled"]'))).toHaveCount(1);
      await expect(attention.and(testPage.locator('[data-kind="blocked"]'))).toHaveCount(1);
      if (mobile) {
        const link = row.getByRole("link", { name: workspaceName!, exact: true });
        const box = await link.boundingBox();
        expect(box?.height).toBeGreaterThanOrEqual(44);
        await assertNoPageOverflow(testPage);
      }

      await testPage.goto("/control/history");
      const folded = testPage.locator('[data-testid="control-history-group"][data-count="3"]');
      await expect(folded.first()).toBeVisible();
      await expect(folded.first().getByTestId("control-history-group-count")).toContainText("3");
      if (mobile) await assertNoPageOverflow(testPage);
    } finally {
      await release();
    }
  });
}
