import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { LONG_TASK_TITLE } from "../../helpers/topbar-fixtures";
import { SessionPage } from "../../pages/session-page";

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
    // The rename control is the element that truncates; the surrounding
    // title crumb is a flex wrapper whose scrollWidth never overflows.
    const title = topbar?.querySelector('[data-testid="task-topbar-title"]') as HTMLElement | null;
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

    // The space policy grants the title all free width and truncates only
    // under pressure, so the fixture title fits untouched at 1280px. Narrow
    // the viewport until the title genuinely competes with the stepper and
    // the tool clusters.
    await testPage.setViewportSize({ width: 900, height: 800 });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const title = testPage.locator('[data-testid="task-topbar"] [data-testid="task-topbar-title"]');
    await expect(title).toHaveText(LONG_TASK_TITLE, { timeout: 10_000 });
    await expect(testPage.getByTestId("layout-preset-trigger")).toBeVisible();

    const metrics = await readDesktopTopbarMetrics(testPage);
    expect(metrics).not.toBeNull();
    if (!metrics) return;

    expect(metrics.documentScrollWidth).toBeLessThanOrEqual(metrics.documentClientWidth + 1);
    // The title yields (clips) instead of pushing chrome out of the bar…
    expect(metrics.titleScrollWidth).toBeGreaterThan(metrics.titleClientWidth + 8);
    // …and never overlaps the stepper or pushes the tools past the bar edge.
    expect(metrics.titleRight).toBeLessThanOrEqual(metrics.stepperLeft + 1);
    expect(metrics.toolsRight).toBeLessThanOrEqual(metrics.topbarRight + 1);
  });
});
