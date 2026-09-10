import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("Completed conversation resume on mobile", () => {
  test("resumes in place with touch-sized actions and no overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Mobile completed conversation ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "Waiting for initial conversation",
    );
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      completedAt: new Date().toISOString(),
    });
    await apiClient.updateTaskState(task.id, "COMPLETED");

    const before = await apiClient.listTaskSessions(task.id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

    const resume = session.completedSessionResumeButton();
    const newAgent = session.completedSessionNewAgentButton();
    await expect(resume).toBeVisible();
    await expect(newAgent).toBeVisible();
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    for (const control of [resume, newAgent]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
    }

    await resume.tap();
    await expect(session.completedSessionBanner()).toHaveCount(0, { timeout: 60_000 });
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible").first()).toBeEditable({
      timeout: 60_000,
    });
    await session.sendMessageViaButton("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });
    await expect
      .poll(
        async () => {
          const current = await apiClient.listTaskSessions(task.id);
          return current.sessions.find((item) => item.id === task.session_id)?.state ?? "MISSING";
        },
        { timeout: 60_000, message: "Waiting for the follow-up turn to become idle" },
      )
      .toBe("WAITING_FOR_INPUT");
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible").first()).toBeEditable({
      timeout: 60_000,
    });

    const after = await apiClient.listTaskSessions(task.id);
    expect(after.sessions).toHaveLength(before.sessions.length);
    expect(after.sessions.find((item) => item.id === task.session_id)?.is_primary).toBe(true);
    expect((await apiClient.getTask(task.id)).state).toBe("COMPLETED");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile completed conversation resume");
  });
});
