import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  restartAndAssertColdWorkspace,
  RETAINED_WORKSPACE_CONTENT,
  RETAINED_WORKSPACE_FILE,
  seedCompletedConversation,
} from "./completed-workspace-restoration-helpers";

test.describe("Completed conversation resume on mobile", () => {
  test("resumes in place with touch-sized actions and no overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const task = await seedCompletedConversation(
      apiClient,
      seedData,
      `Mobile completed conversation ${Date.now()}`,
    );
    if (!task.session_id) throw new Error("completed task has no session_id");

    const before = await apiClient.listTaskSessions(task.id);
    await restartAndAssertColdWorkspace(backend, apiClient, task.id, task.session_id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

    await testPage.getByRole("button", { name: "Files" }).tap();
    const fileNode = session.fileTreeNode(RETAINED_WORKSPACE_FILE);
    await expect(fileNode).toBeVisible({ timeout: 60_000 });
    await fileNode.tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(
      viewer
        .getByTestId("mobile-file-viewer-content")
        .locator(".cm-line")
        .filter({ hasText: RETAINED_WORKSPACE_CONTENT }),
    ).toBeVisible();
    await viewer.getByRole("button", { name: "Close" }).tap();
    await testPage.getByRole("button", { name: "Chat" }).tap();

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
