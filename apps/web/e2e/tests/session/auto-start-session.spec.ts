import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Auto-start session on task page", () => {
  test("navigating to a task with no session auto-starts one", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Create a task with an agent profile but no existing session — auto-start
    // will launch one. The description becomes the initial prompt.
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Start Session Task", {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      agent_profile_id: seedData.agentProfileId,
    });

    // Navigate directly to the task page — no session exists yet
    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);

    // The auto-start hook should detect no sessions and launch one automatically
    await session.waitForLoad();

    // Wait for agent to complete — confirms session was started and ran successfully
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });
  });
});
