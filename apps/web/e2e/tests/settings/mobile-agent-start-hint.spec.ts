import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/git-helper";

// Mobile variant of the composer agent-start hint: the resume-skipped gate
// and the hint render through the shared responsive TaskPageContent
// (session-mobile-layout renders TaskChatPanel), so a recovered-idle session
// must show the hint on a phone viewport and a sent message must start the
// agent. Mirrors the desktop restart-resume flow in
// settings/prevent-auto-start-on-open.spec.ts.

test.describe("Mobile agent-start hint", () => {
  test("recovered-idle session shows the composer hint and a message starts the agent", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    try {
      await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mobile Agent Start Hint Task",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );

      // Let the agent finish its first turn so the session ends idle with a
      // resume token (the post-restart needs_resume shape).
      const session = await openTaskSession(testPage, "Mobile Agent Start Hint Task");
      await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });

      // Restart the backend and reload — the resume-skipped gate must keep
      // the agent stopped and the composer hint must replace the footer
      // Start agent button.
      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();

      await expect(testPage.getByTestId("composer-agent-start-hint")).toBeVisible({
        timeout: 60_000,
      });
      await expect(testPage.getByTestId("session-resume-start-button")).toHaveCount(0);
      await expect(testPage.getByText("Resumed agent", { exact: false })).toHaveCount(0);

      // Sending a message is the explicit start: the agent resumes and
      // keeps working. Touch layouts don't submit on Ctrl/Cmd+Enter, so use
      // the Send-button path like the other mobile specs. Index 1: the first
      // "simple mock response" from the pre-restart turn is still in the
      // transcript, so the post-send reply must be the SECOND match.
      await session.sendMessageViaButton("/e2e:simple-message");
      await expect(testPage.getByText("Resumed agent", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });
});
