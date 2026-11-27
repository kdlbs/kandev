import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const HINT = /without the leading slash/i;
const MOCK_OUTPUT = "This is a simple mock response for e2e testing.";

test.describe("Empty-turn notice", () => {
  test.describe.configure({ retries: 1 });

  test("shows a slash-command hint when a /command produces no output", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Empty Turn Hint",
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

    await test.step("does not show a hint when the turn produces output", async () => {
      await expect(session.chat).toContainText(MOCK_OUTPUT, { timeout: 30_000 });
      await session.waitForChatIdle();
      await expect(session.chat).not.toContainText(HINT);
    });

    // The notice is live-only: submit after the chat is ready to receive the
    // completion event, independently of how long the initial navigation took.
    await session.sendMessage("/e2e:empty-turn");
    await expect(session.chat).toContainText(HINT, { timeout: 30_000 });
    await session.waitForChatIdle();
  });
});
