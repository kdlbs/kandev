import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  restartAndAssertColdWorkspace,
  RETAINED_WORKSPACE_CONTENT,
  RETAINED_WORKSPACE_FILE,
  seedCompletedConversation,
} from "./completed-workspace-restoration-helpers";

test.describe("Completed conversation resume", () => {
  test("resumes the selected completed conversation and sends a follow-up in place", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const task = await seedCompletedConversation(
      apiClient,
      seedData,
      `Completed conversation ${Date.now()}`,
    );
    if (!task.session_id) throw new Error("completed task has no session_id");
    const before = await apiClient.listTaskSessions(task.id);
    const primary = before.sessions.find((session) => session.is_primary);
    expect(primary?.id).toBe(task.session_id);
    await restartAndAssertColdWorkspace(backend, apiClient, task.id, task.session_id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const banner = session.completedSessionBanner();
    await expect(banner).toBeVisible({ timeout: 30_000 });
    await expect(session.completedSessionResumeButton()).toBeVisible();
    await expect(session.completedSessionNewAgentButton()).toBeVisible();

    await session.clickTab("Files");
    const fileNode = session.fileTreeNode(RETAINED_WORKSPACE_FILE);
    await expect(fileNode).toBeVisible({ timeout: 60_000 });
    await fileNode.click();
    const viewer = testPage.locator(".monaco-editor:visible").first();
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(
      viewer.locator(".view-lines").filter({ hasText: RETAINED_WORKSPACE_CONTENT }),
    ).toBeVisible();
    await session.clickSessionChatTab();

    // Opening historical work is passive. Reloading must not launch a new
    // execution or replace the selected session.
    await testPage.reload();
    await session.showSessionContext();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });
    await session.clickTab("Files");
    await expect(session.fileTreeNode(RETAINED_WORKSPACE_FILE)).toBeVisible({ timeout: 60_000 });
    await session.clickSessionChatTab();
    const afterReload = await apiClient.listTaskSessions(task.id);
    expect(afterReload.sessions).toHaveLength(before.sessions.length);
    expect(afterReload.sessions.find((item) => item.is_primary)?.id).toBe(task.session_id);

    // Recreate the cold runtime after workspace restoration. The next open
    // must persist provider identity before explicit Resume uses it.
    await backend.restart();
    await testPage.reload();
    await session.showSessionContext();
    await session.clickTab("Files");
    await expect(session.fileTreeNode(RETAINED_WORKSPACE_FILE)).toBeVisible({ timeout: 60_000 });
    await session.clickSessionChatTab();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

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
