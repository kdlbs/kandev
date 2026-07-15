import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

async function seedTaskWithPlan(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: planScript("## Plan\n\n1. Build the toolbar implement action"),
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
  await openPlanPanel(session);
  return { taskId: task.id, sessionId: task.session_id!, session };
}

async function openPlanPanel(session: SessionPage) {
  if (await session.planPanel.isVisible()) return;
  await session.togglePlanMode();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

test.describe("Plan toolbar implement", () => {
  test.describe.configure({ retries: 1 });

  test("saves unsaved plan text, starts implementation, and stays disabled after refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { taskId, sessionId, session } = await seedTaskWithPlan(
      testPage,
      apiClient,
      seedData,
      "Plan toolbar implement desktop",
    );

    const toolbarButton = testPage.getByTestId("plan-toolbar-implement-button");
    await expect(toolbarButton).toBeVisible({ timeout: 10_000 });

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
    expect(toolbarSpacing?.controlHeight).toBeLessThanOrEqual(26);
    expect(toolbarSpacing?.top).toBeGreaterThanOrEqual(1);
    expect(toolbarSpacing?.bottom).toBeGreaterThanOrEqual(1);

    const uniqueEdit = `toolbar marker ${Date.now()}`;
    await session.planEditor().click();
    await testPage.keyboard.press(process.platform === "darwin" ? "Meta+End" : "Control+End");
    await testPage.keyboard.type(`\n\n${uniqueEdit}`);

    await toolbarButton.click();

    await expect
      .poll(
        async () => {
          const plan = await apiClient.getTaskPlan(taskId);
          return {
            hasEdit: plan?.content.includes(uniqueEdit) ?? false,
            markerSession: plan?.implementation_started_session_id ?? null,
          };
        },
        { timeout: 30_000 },
      )
      .toEqual({ hasEdit: true, markerSession: sessionId });

    const { messages } = await apiClient.listSessionMessages(sessionId);
    const latestUserMessage = messages.filter((m) => m.author_type === "user").pop();
    expect(latestUserMessage?.content).toContain("Implement the plan");
    await expect(toolbarButton).toBeVisible({ timeout: 10_000 });
    await expect(toolbarButton).toBeDisabled();

    await testPage.getByTestId("plan-toolbar-implement-control").hover();
    await expect(
      testPage.getByText("This plan has already been sent for implementation."),
    ).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await openPlanPanel(session);
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeDisabled();
  });
});
