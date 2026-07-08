import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const LONG_TASK_TITLE =
  'Issue #1625: [Bug]: Config-session MCP advertises list_executors but backend returns "Unknown action: mcp.list_executors"';

type DesktopTopbarMetrics = {
  documentClientWidth: number;
  documentScrollWidth: number;
  stepperLeft: number;
  titleClientWidth: number;
  titleRight: number;
  titleScrollWidth: number;
  titleWidth: number;
  toolsRight: number;
  topbarRight: number;
  topbarWidth: number;
};

async function readDesktopTopbarMetrics(page: Page): Promise<DesktopTopbarMetrics | null> {
  return page.evaluate(() => {
    const topbar = document.querySelector('[data-testid="task-topbar"]') as HTMLElement | null;
    const title = topbar?.querySelector('[aria-current="page"]') as HTMLElement | null;
    const stepper = topbar?.querySelector('[data-testid="workflow-stepper"]') as HTMLElement | null;
    const tools = topbar?.querySelector('[aria-label="Task tools"]') as HTMLElement | null;
    if (!topbar || !title || !stepper || !tools) return null;

    const topbarRect = topbar.getBoundingClientRect();
    const titleRect = title.getBoundingClientRect();
    const stepperRect = stepper.getBoundingClientRect();
    const toolsRect = tools.getBoundingClientRect();

    return {
      documentClientWidth: document.documentElement.clientWidth,
      documentScrollWidth: document.documentElement.scrollWidth,
      stepperLeft: stepperRect.left,
      titleClientWidth: title.clientWidth,
      titleRight: titleRect.right,
      titleScrollWidth: title.scrollWidth,
      titleWidth: titleRect.width,
      toolsRight: toolsRect.right,
      topbarRight: topbarRect.right,
      topbarWidth: topbarRect.width,
    };
  });
}

test.describe("Task topbar long title layout", () => {
  test("truncates long task titles without overlapping workflow or tool controls", async ({
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

    const title = testPage.locator('[data-testid="task-topbar"] [aria-current="page"]');
    await expect(title).toHaveText(LONG_TASK_TITLE, { timeout: 10_000 });
    await expect(testPage.getByTestId("layout-preset-trigger")).toBeVisible();

    const metrics = await readDesktopTopbarMetrics(testPage);
    expect(metrics).not.toBeNull();
    if (!metrics) return;

    expect(metrics.documentScrollWidth).toBeLessThanOrEqual(metrics.documentClientWidth + 1);
    expect(metrics.titleScrollWidth).toBeGreaterThan(metrics.titleClientWidth + 8);
    expect(metrics.titleWidth).toBeLessThanOrEqual(metrics.topbarWidth * 0.6);
    expect(metrics.titleRight).toBeLessThanOrEqual(metrics.stepperLeft + 1);
    expect(metrics.toolsRight).toBeLessThanOrEqual(metrics.topbarRight + 1);
  });
});
