import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const LONG_TASK_TITLE =
  'Issue #1625: [Bug]: Config-session MCP advertises list_executors but backend returns "Unknown action: mcp.list_executors"';

type MobileTopbarMetrics = {
  actionsLeft: number;
  actionsRight: number;
  documentClientWidth: number;
  documentScrollWidth: number;
  headerLeft: number;
  headerRight: number;
  titleClientWidth: number;
  titleRight: number;
  titleScrollWidth: number;
};

async function readMobileTopbarMetrics(
  page: Page,
  taskTitle: string,
): Promise<MobileTopbarMetrics | null> {
  return page.evaluate((titleText) => {
    const header = Array.from(document.querySelectorAll("header")).find((node) =>
      node.textContent?.includes(titleText),
    ) as HTMLElement | undefined;
    const title = Array.from(header?.querySelectorAll("span") ?? []).find(
      (node) => node.textContent?.trim() === titleText,
    ) as HTMLElement | undefined;
    const actions = header?.children.item(1) as HTMLElement | null | undefined;
    if (!header || !title || !actions) return null;

    const headerRect = header.getBoundingClientRect();
    const titleRect = title.getBoundingClientRect();
    const actionsRect = actions.getBoundingClientRect();

    return {
      actionsLeft: actionsRect.left,
      actionsRight: actionsRect.right,
      documentClientWidth: document.documentElement.clientWidth,
      documentScrollWidth: document.documentElement.scrollWidth,
      headerLeft: headerRect.left,
      headerRight: headerRect.right,
      titleClientWidth: title.clientWidth,
      titleRight: titleRect.right,
      titleScrollWidth: title.scrollWidth,
    };
  }, taskTitle);
}

test.describe("Mobile task topbar long title layout", () => {
  test("truncates long task titles without pushing mobile actions off-screen", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      LONG_TASK_TITLE,
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

    const header = testPage.locator("header").filter({ hasText: LONG_TASK_TITLE }).first();
    await expect(header).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("mobile-session-menu")).toBeVisible();

    const metrics = await readMobileTopbarMetrics(testPage, LONG_TASK_TITLE);
    expect(metrics).not.toBeNull();
    if (!metrics) return;

    expect(metrics.documentScrollWidth).toBeLessThanOrEqual(metrics.documentClientWidth + 1);
    expect(metrics.titleScrollWidth).toBeGreaterThan(metrics.titleClientWidth + 8);
    expect(metrics.titleRight).toBeLessThanOrEqual(metrics.actionsLeft + 1);
    expect(metrics.headerLeft).toBeGreaterThanOrEqual(0);
    expect(metrics.actionsRight).toBeLessThanOrEqual(metrics.headerRight + 1);
  });
});
