import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { expectControlHeight } from "../../helpers/control-sizing";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

async function seedCompletedSession(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Control sizing completed session task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    sessionId: task.session_id,
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.completedSessionNewAgentButton()).toBeVisible({ timeout: 15_000 });
  return session;
}

test.describe("Task control sizing", () => {
  test("completed-session New Agent uses the standard desktop height", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedCompletedSession(testPage, apiClient, seedData);

    await expectControlHeight(session.completedSessionNewAgentButton(), 28);
  });

  test("task creation keeps touch sizing until the md breakpoint", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const branchesOrWorkflowLoaded = waitForHttp(testPage, "GET", /\/workflows|\/repositories/);
    await kanban.createTaskButton.first().click();
    await branchesOrWorkflowLoaded;
    await testPage.setViewportSize({ width: 700, height: 900 });

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-title-input").fill("Control sizing task");
    await dialog.getByTestId("task-description-input").fill("Check task action geometry");

    const startButton = dialog.getByTestId("submit-start-agent");
    await expect(startButton).toBeEnabled();
    await expectControlHeight(startButton, 44);
  });
});
