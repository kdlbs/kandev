// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

async function seedMobileTaskWithPlan(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Plan toolbar implement mobile",
    seedData.agentProfileId,
    {
      description: planScript("## Plan\n\n1. Build the mobile toolbar implement action"),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  await session.waitForChatIdle({ timeout: 45_000 });
  await openMobilePlanPanel(testPage, session);
  return { taskId: task.id, sessionId: task.session_id!, session };
}

async function openMobilePlanPanel(testPage: Page, session: SessionPage) {
  await session.togglePlanMode();
  await testPage.getByRole("button", { name: "Plan" }).tap();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

test.describe("mobile: Plan toolbar implement", () => {
  test.describe.configure({ retries: 1 });

  test("marks the plan implemented and remains disabled after refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { taskId, sessionId, session } = await seedMobileTaskWithPlan(
      testPage,
      apiClient,
      seedData,
    );

    const toolbarButton = testPage.getByTestId("plan-toolbar-implement-button");
    await expect(toolbarButton).toBeVisible({ timeout: 10_000 });
    await expect(toolbarButton).toBeInViewport();
    await expect(toolbarButton).toBeEnabled({ timeout: 10_000 });

    const toolbarSpacing = await testPage
      .getByTestId("plan-toolbar-implement-control")
      .evaluate((control) => {
        const toolbar = control.closest(".border-b");
        if (!(toolbar instanceof HTMLElement)) return null;
        const controlRect = control.getBoundingClientRect();
        const toolbarRect = toolbar.getBoundingClientRect();
        return {
          top: controlRect.top - toolbarRect.top,
          bottom: toolbarRect.bottom - controlRect.bottom,
          toolbarHeight: toolbarRect.height,
          controlHeight: controlRect.height,
        };
      });
    expect(toolbarSpacing?.toolbarHeight).toBe(30);
    expect(toolbarSpacing?.controlHeight).toBe(22);
    expect(toolbarSpacing?.top).toBeGreaterThanOrEqual(1);
    expect(toolbarSpacing?.bottom).toBeGreaterThanOrEqual(1);

    const overflow = await testPage.evaluate(() => {
      const root = document.scrollingElement ?? document.documentElement;
      return root.scrollWidth > root.clientWidth + 1;
    });
    expect(overflow).toBe(false);

    await toolbarButton.tap();

    await expect
      .poll(
        async () => {
          const plan = await apiClient.getTaskPlan(taskId);
          return plan?.implementation_started_session_id ?? null;
        },
        { timeout: 30_000 },
      )
      .toBe(sessionId);

    await expect(toolbarButton).toBeVisible({ timeout: 10_000 });
    await expect(toolbarButton).toBeDisabled();

    await testPage.reload();
    await session.waitForLoad();
    await openMobilePlanPanel(testPage, session);
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeDisabled();
  });
});
