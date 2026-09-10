import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  failNextWorkspaceRestore,
  restartAndAssertColdWorkspace,
  RETAINED_WORKSPACE_CONTENT,
  RETAINED_WORKSPACE_FILE,
  seedCompletedConversation,
} from "./completed-workspace-restoration-helpers";

test.describe("Completed workspace restoration", () => {
  test.describe.configure({ retries: 1 });

  test("restores a cold workspace and recovers a bounded failure", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(240_000);
    const task = await seedCompletedConversation(
      apiClient,
      seedData,
      `Completed workspace restoration ${Date.now()}`,
    );
    if (!task.session_id) throw new Error("completed task has no session_id");

    const beforeSessions = await apiClient.listTaskSessions(task.id);
    const beforeMessages = await apiClient.listSessionMessages(task.session_id);
    await restartAndAssertColdWorkspace(backend, apiClient, task.id, task.session_id);

    const failure = await failNextWorkspaceRestore(
      testPage,
      task.id,
      task.session_id,
      "workspace restore failed for e2e",
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

    await session.clickTab("Files");
    const workspaceUnavailable = session.files.getByTestId("workspace-unavailable");
    await expect(workspaceUnavailable).toBeVisible({ timeout: 30_000 });
    await expect(session.files.getByTestId("file-tree-waiting")).toHaveCount(0);
    await expect(session.recoveryError()).toHaveCount(0);
    expect(failure.wasConsumed()).toBe(true);

    await workspaceUnavailable.getByText("Technical details", { exact: true }).click();
    await expect(workspaceUnavailable.locator("pre")).toContainText(
      "workspace restore failed for e2e",
    );

    const retry = workspaceUnavailable.getByTestId("workspace-retry");
    const retryBox = await retry.boundingBox();
    expect(retryBox, "workspace retry has no rendered hitbox").not.toBeNull();
    expect(retryBox?.height ?? 0).toBeGreaterThanOrEqual(32);
    await retry.click();

    const fileNode = session.fileTreeNode(RETAINED_WORKSPACE_FILE);
    await expect(fileNode).toBeVisible({ timeout: 60_000 });
    await fileNode.click();
    const viewer = testPage.locator(".monaco-editor:visible").first();
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(
      viewer.locator(".view-lines").filter({ hasText: RETAINED_WORKSPACE_CONTENT }),
    ).toBeVisible();

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    await expect(session.changes.getByTestId("workspace-unavailable")).toHaveCount(0);

    await session.clickTab("Terminal");
    await expect(session.terminal).toBeVisible();
    await session.typeInTerminal("printf 'RESTORED_WORKSPACE_SHELL\\n'");
    await session.expectTerminalHasText("RESTORED_WORKSPACE_SHELL");

    const workspaceStatus = await apiClient.wsRequest<{
      state: string;
      is_agent_running: boolean;
    }>("task.session.status", { task_id: task.id, session_id: task.session_id });
    expect(workspaceStatus.state).toBe("COMPLETED");
    expect(workspaceStatus.is_agent_running).toBe(false);

    const afterWorkspace = await apiClient.listTaskSessions(task.id);
    const afterWorkspaceMessages = await apiClient.listSessionMessages(task.session_id);
    expect(afterWorkspace.sessions).toHaveLength(beforeSessions.sessions.length);
    expect(afterWorkspace.sessions.find((item) => item.id === task.session_id)?.state).toBe(
      "COMPLETED",
    );
    expect(afterWorkspaceMessages.messages).toHaveLength(beforeMessages.messages.length);
    expect(await apiClient.getTask(task.id)).toMatchObject({
      id: task.id,
      state: "COMPLETED",
      primary_session_id: task.session_id,
    });
    await expect(session.recoveryError()).toHaveCount(0);

    await prCapture.screenshot("completed-workspace-restoration-desktop", {
      caption: "Desktop completed task with restored workspace panels",
    });

    await testPage.reload();
    await session.showSessionContext();
    await session.clickTab("Files");
    await expect(session.fileTreeNode(RETAINED_WORKSPACE_FILE)).toBeVisible({ timeout: 60_000 });
    await session.clickSessionChatTab();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

    const finalSessions = await apiClient.listTaskSessions(task.id);
    const finalMessages = await apiClient.listSessionMessages(task.session_id);
    expect(finalSessions.sessions).toHaveLength(beforeSessions.sessions.length);
    expect(finalMessages.messages).toHaveLength(beforeMessages.messages.length);
    await assertNoDocumentHorizontalOverflow(testPage, "completed workspace restoration");
  });
});
