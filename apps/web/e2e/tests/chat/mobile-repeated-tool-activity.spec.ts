// Filename starts with "mobile-" so the mobile-chrome project exercises this
// narrow-touch chat surface.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

function repeatedToolMetadata(i: number) {
  return {
    status: "complete",
    tool_call_id: `tc-repeat-${i}`,
    normalized: {
      shell_exec: {
        command: "gh pr checks",
        output: { exit_code: 0, stdout: "1" },
      },
    },
  };
}

test.describe("mobile: repeated tool activity", () => {
  test("compacts repeated terminal commands inside an expanded turn group", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Repeated Tool Activity", {
      description: "seeded repeated tool activity",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });

    for (let i = 0; i < 6; i++) {
      await apiClient.seedSessionMessage(sessionId, {
        type: "tool_execute",
        content: "gh pr checks",
        metadata: repeatedToolMetadata(i),
      });
    }

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    const groupToggle = chat.getByRole("button", { name: /6\s+tool calls/i });
    await expect(groupToggle).toBeVisible({ timeout: 15_000 });
    await groupToggle.click();

    await expect(chat.getByTestId("repeated-tool-summary")).toContainText(
      "4 repeated identical terminal commands hidden",
    );
  });
});
