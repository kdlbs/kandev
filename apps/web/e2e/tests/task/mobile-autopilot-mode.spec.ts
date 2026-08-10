import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

useRegularMode();

function parentQuestionScript(): string {
  const args = JSON.stringify({
    questions: [
      {
        id: "risk",
        title: "Risk",
        prompt: "Choose the safe path before I continue.",
        options: [
          { option_id: "safe", label: "Safe path", description: "Use the conservative path." },
          { option_id: "fast", label: "Fast path", description: "Use the quicker path." },
        ],
      },
    ],
    context: "The implementation has two valid paths.",
  });
  return `e2e:mcp:kandev:ask_parent_question_kandev(${args})`;
}

async function waitForState(apiClient: ApiClient, taskId: string, state: string): Promise<void> {
  await expect
    .poll(async () => (await apiClient.listTaskSessions(taskId)).sessions[0]?.state ?? "", {
      timeout: 60_000,
      message: `task ${taskId} should reach ${state}`,
    })
    .toBe(state);
}

test.describe("Mobile task autopilot", () => {
  test("keeps the control, chip, and waiting indicators inside the viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();
    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog.getByTestId("autopilot-toggle-row")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile autopilot create dialog");
    await createDialog.getByRole("button", { name: "Cancel", exact: true }).tap();

    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Autopilot Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const child = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Autopilot Child",
      seedData.agentProfileId,
      {
        description: parentQuestionScript(),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        parent_id: parent.id,
        workspace_mode: "inherit_parent",
        autopilot: true,
      },
    );
    const childTask = await apiClient.getTask(child.id);
    expect(childTask.autopilot).toBe(true);

    await testPage.goto(`/t/${parent.id}`);
    const parentSession = new SessionPage(testPage);
    await parentSession.waitForLoad();
    await waitForState(apiClient, child.id, "WAITING_FOR_INPUT");
    await parentSession.mobileSessionMenu.click();
    const sheet = testPage.getByRole("dialog");
    const childRow = sheet.getByTestId("sidebar-task-item").filter({
      hasText: "Mobile Autopilot Child",
    });
    await expect(childRow).toBeVisible({ timeout: 30_000 });
    await expect(childRow.getByTestId("task-autopilot-icon")).toBeVisible();
    await expect(childRow.getByTestId("task-state-waiting-for-input")).toBeVisible({
      timeout: 15_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile autopilot task switcher");

    await expect
      .poll(async () => (await apiClient.listTaskSessions(child.id)).sessions[0]?.state ?? "", {
        timeout: 30_000,
        message: "the parent answer should resume the mobile child",
      })
      .not.toBe("WAITING_FOR_INPUT");

    await testPage.goto(`/t/${child.id}`);
    const childSession = new SessionPage(testPage);
    await childSession.waitForLoad();
    await expect(childSession.chatStatusBar().getByTestId("chat-autopilot-chip")).toBeVisible({
      timeout: 15_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile autopilot chat");
  });
});
