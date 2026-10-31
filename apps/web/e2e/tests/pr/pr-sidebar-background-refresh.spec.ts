import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  PR_SIDEBAR_REFRESH,
  publishPRSidebarBackgroundRefresh,
  recordGatewayFrames,
  seedPRSidebarBackgroundRefresh,
} from "./pr-sidebar-background-refresh-fixtures";

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.8
test("updates an inactive desktop task row from a newer task summary", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const seed = await seedPRSidebarBackgroundRefresh(apiClient, seedData);
  const ws = watchWs(testPage);
  const frames = recordGatewayFrames(testPage);

  await testPage.goto(`/t/${seed.navigationTaskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const targetRow = session.sidebarTaskItem(PR_SIDEBAR_REFRESH.targetTitle);
  await expect(targetRow).toBeVisible();
  const icon = targetRow.getByTestId(`pr-task-icon-${seed.targetTaskId}`);
  await expect(icon).toBeVisible();
  await expect(icon).toHaveAttribute("data-pr-state", /^(open|Open)$/);

  frames.reset();
  const summary = ws.waitForEvent("task.status_summary.updated", {
    where: (payload) => {
      if (payload.task_id !== seed.targetTaskId) return false;
      const summaryPayload = payload.status_summary as
        | { pull_request?: { state?: string } }
        | undefined;
      return summaryPayload?.pull_request?.state === "merged";
    },
  });
  await publishPRSidebarBackgroundRefresh(apiClient, seedData, seed, "merged");
  await summary;

  await expect(icon).toHaveAttribute("data-pr-state", /^(merged|Merged)$/);
  await expect(testPage).toHaveURL(new RegExp(`/t/${seed.navigationTaskId}`));
  expect(frames.syncRequestsFor(seed.targetTaskId)).toHaveLength(0);
});
