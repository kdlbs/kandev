import { test, expect } from "../../fixtures/test-base";
import { firstSessionId, waitForAgentMessage } from "../../helpers/session";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

/**
 * An agent-started turn (here, the follow-up to a provider notification) must
 * reach completion. The recovered seq 1173 frame was a metadata-only
 * tool_call_update for a tool that had already finished; before the fix it
 * entered the idle finalizer's active set, so the turn never completed and the
 * queued message was never delivered.
 */
test.describe("queued message after an agent-started follow-up", () => {
  test.describe.configure({ retries: 1 });

  test("delivers a message queued during the follow-up without user action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Queued message after follow-up",
      seedData.agentProfileId,
      {
        description: "/e2e:async-followup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const sessionId = await firstSessionId(apiClient, task.id);

    // The prompt turn completes, then the agent starts its own follow-up.
    await waitForAgentMessage(
      apiClient,
      sessionId,
      "Foreground response before the async follow-up.",
    );
    await expect(
      testPage.getByText("Foreground response before the async follow-up."),
    ).toBeVisible();
    await waitForAgentMessage(apiClient, sessionId, "Background follow-up started.");
    await expect(testPage.getByText("Background follow-up started.")).toBeVisible();

    // Queue while that turn runs. Auto-run is on by default.
    await waitForComposerQueueMode(testPage);
    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, 'e2e:message("queued follow-up delivered")');
    await testPage.getByTestId("submit-message-button").click();
    const queueIdentity = await apiClient.getQueueSessionIdentity(task.id, sessionId);
    await expect
      .poll(() => apiClient.getQueueStatus(queueIdentity).then((status) => status.count), {
        message: "the submitted follow-up should be present in the durable queue",
      })
      .toBeGreaterThan(0);
    await expect(testPage.getByTestId("queue-chip")).toBeVisible();
    await waitForAgentMessage(apiClient, sessionId, "queued follow-up delivered", 90_000);
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await session.waitForChatIdle({ timeout: 30_000 });
  });
});
