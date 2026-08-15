// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const PLAN_STEP = "Build the mobile toolbar implement action";

async function seedMobileTaskWithPlan(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Plan toolbar implement mobile",
    seedData.agentProfileId,
    {
      description: planScript(`## Plan\n\n1. ${PLAN_STEP}`),
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
  await session.composerReady();
  await openMobilePlanPanel(testPage, session);
  // The implement button is disabled until the panel actually holds the plan.
  // Two transports can deliver it -- the `task.plan.created` notification
  // (which marks the store loaded, so mounting the panel fetches nothing) or a
  // `task.plan.get` on mount when that notification lost the race -- so there
  // is no single frame to wait for here. Wait on the plan *content*, which is
  // the button's real precondition, instead of on the button's own pending
  // state. Everything downstream of this does have one signal, and uses it.
  await expect(session.planPanel).toContainText(PLAN_STEP);
  return { sessionId: task.session_id!, session };
}

async function openMobilePlanPanel(testPage: Page, session: SessionPage) {
  await session.togglePlanMode();
  await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

test.describe("mobile: Plan toolbar implement", () => {
  test("marks the plan implemented and remains disabled after refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // Must be armed before the first navigation: Playwright only reports
    // sockets opened after the listener is attached.
    const ws = watchWs(testPage);
    const { sessionId, session } = await seedMobileTaskWithPlan(testPage, apiClient, seedData);

    const toolbarButton = testPage.getByTestId("plan-toolbar-implement-button");
    await expect(toolbarButton).toBeVisible({ timeout: 10_000 });
    await expect(toolbarButton).toBeEnabled();
    await expect(toolbarButton).toBeInViewport();

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

    // Tapping implement sends `task.plan.implementation_started` over the WS.
    // Its reply is what stamps the plan and re-disables the button, so wait on
    // the reply rather than polling the backend for 30s for the same fact.
    const implementationStarted = ws.waitForResponse("task.plan.implementation_started");
    await toolbarButton.tap();
    const stamped = await implementationStarted;
    expect(stamped.payload.implementation_started_session_id).toBe(sessionId);

    await expect(toolbarButton).toBeVisible();
    await expect(toolbarButton).toBeDisabled();

    // After a reload the store is empty, so remounting the panel does refetch:
    // `task.plan.get` carries the stamp that keeps the button disabled.
    const planReloaded = ws.waitForResponse("task.plan.get");
    await testPage.reload();
    await session.waitForLoad();
    await openMobilePlanPanel(testPage, session);
    await planReloaded;
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeVisible();
    await expect(testPage.getByTestId("plan-toolbar-implement-button")).toBeDisabled();
  });
});
