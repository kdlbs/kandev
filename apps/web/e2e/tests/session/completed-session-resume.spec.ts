import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

async function seedCompletedConversation(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Completed conversation ${Date.now()}`,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for initial conversation");
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    sessionId: task.session_id,
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.updateTaskState(task.id, "COMPLETED");
  return task;
}

test.describe("Completed conversation resume", () => {
  test("resumes the selected completed conversation and sends a follow-up in place", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await seedCompletedConversation(apiClient, seedData);
    const before = await apiClient.listTaskSessions(task.id);
    const primary = before.sessions.find((session) => session.is_primary);
    expect(primary?.id).toBe(task.session_id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const banner = session.completedSessionBanner();
    await expect(banner).toBeVisible({ timeout: 30_000 });
    await expect(session.completedSessionResumeButton()).toBeVisible();
    await expect(session.completedSessionNewAgentButton()).toBeVisible();

    // Opening historical work is passive. Reloading must not launch a new
    // execution or replace the selected session.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });
    const afterReload = await apiClient.listTaskSessions(task.id);
    expect(afterReload.sessions).toHaveLength(before.sessions.length);
    expect(afterReload.sessions.find((item) => item.is_primary)?.id).toBe(task.session_id);

    await session.completedSessionResumeButton().click();
    await expect(session.completedSessionBanner()).toHaveCount(0, { timeout: 60_000 });
    await expect
      .poll(
        async () => {
          const current = await apiClient.listTaskSessions(task.id);
          return current.sessions.find((item) => item.id === task.session_id)?.state ?? "MISSING";
        },
        { timeout: 60_000, message: "Waiting for the resumed conversation to become idle" },
      )
      .toBe("WAITING_FOR_INPUT");
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible").first()).toBeEditable({
      timeout: 60_000,
    });

    const afterResume = await apiClient.listTaskSessions(task.id);
    expect(afterResume.sessions).toHaveLength(before.sessions.length);
    expect(afterResume.sessions.find((item) => item.id === task.session_id)?.state).toBe(
      "WAITING_FOR_INPUT",
    );

    await session.sendMessage("/e2e:simple-message");
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

    const afterFollowUp = await apiClient.listTaskSessions(task.id);
    const taskAfterFollowUp = await apiClient.getTask(task.id);
    expect(afterFollowUp.sessions).toHaveLength(before.sessions.length);
    expect(afterFollowUp.sessions.find((item) => item.id === task.session_id)?.is_primary).toBe(
      true,
    );
    expect(taskAfterFollowUp.primary_session_id).toBe(task.session_id);
    expect(taskAfterFollowUp.state).toBe("COMPLETED");
    const messages = await apiClient.listSessionMessages(task.session_id);
    expect(messages.messages.filter((message) => message.author_type === "user")).toHaveLength(2);
    expect(
      messages.messages.filter(
        (message) =>
          message.author_type === "agent" && message.content.includes("simple mock response"),
      ),
    ).toHaveLength(2);
    await assertNoDocumentHorizontalOverflow(testPage, "completed conversation resume");
  });
});
