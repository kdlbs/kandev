import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";

test.describe("Advisor feedback messages", () => {
  test("renders persisted advisor feedback in the session feed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Advisor feedback feed",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });

    await apiClient.seedSessionMessage(task.session_id, {
      type: "advisor_feedback",
      content: "Good point. Verify the ACP conversion path.",
      metadata: { source: "advisor", severity: "concern" },
    });

    await testPage.reload();
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText("OMP Advisor Feedback")).toBeVisible();
    await expect(chat.getByText("Good point. Verify the ACP conversion path.")).toBeVisible();
  });
});
