// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

// The mobile surface uses the same recovery state as desktop. This test keeps
// the touch path in the real session chat while proving that backend restart
// does not replace a surviving worktree agent or expose a recovery action.
test.describe("mobile: agent survival across backend restart", () => {
  test.describe.configure({ retries: 1 });

  test("a worktree session's in-flight turn survives a graceful backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mobile Agent Survival Restart Task",
        seedData.agentProfileId,
        {
          description: "/slow 12",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toBeVisible({
        timeout: 30_000,
      });
      await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });

      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();

      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);
      await expect(session.recoveryFreshButton()).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toHaveCount(0);
      await expect(session.chat.getByText("Slow response complete", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });

      await session.sendMessageViaButton("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
      await assertNoDocumentHorizontalOverflow(testPage, "mobile agent survival recovery");
    } finally {
      await releaseFeature();
    }
  });
});
