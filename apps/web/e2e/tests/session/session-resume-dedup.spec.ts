import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";

test.describe("Session resume boot-message dedup", () => {
  // Test restarts the backend multiple times — can be flaky under CI load.
  test.describe.configure({ retries: 1 });

  test("only the most recent 'Resumed agent' boot message is visible", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // 1. Create the task and wait for the initial agent turn to finish.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume Dedup Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, task.id);
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 15_000 });

    // 2. Initial "Started agent Mock" row should be visible exactly once.
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 3. Restart the backend twice to produce two "Resumed agent" boot
    //    messages. The first must be hidden by the dedup; only the second
    //    (most recent) should remain visible.
    for (let i = 0; i < 2; i++) {
      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();
      // Auto-resume + agent turn completion can be slow under CI load, and
      // can transit through the env-setup-failed recovery state.
      await session.waitForChatIdle({ timeout: 60_000 });
      await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
    }

    // 4. Key assertion: despite two resumes, only the last "Resumed agent"
    //    row should be rendered.
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 5. The original "Started agent" row must still be present — dedup must
    //    not affect non-resuming boot messages.
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 6. Agent interaction still works after dedup. After two restart+reload+
    //    auto-resume cycles, the editor's submit handler can race with WS
    //    resubscription: the keystroke is accepted by the optimistic input
    //    but the message never reaches the backend, so neither the user
    //    prompt nor the agent reply ever appear (no reload can recover them
    //    because they were never persisted). Verify the user prompt actually
    //    echoes in the chat; if not, the send was dropped, so re-idle and
    //    resend once before asserting the reply.
    const followupPrompt = "/e2e:simple-message";
    await session.sendMessage(followupPrompt);
    const followupEcho = session.chat.getByText(followupPrompt, { exact: false }).nth(1);
    try {
      await expect(followupEcho).toBeVisible({ timeout: 10_000 });
    } catch {
      await session.waitForChatIdle({ timeout: 30_000 });
      await session.sendMessage(followupPrompt);
      await expect(followupEcho).toBeVisible({ timeout: 15_000 });
    }
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });
});
