import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  focusTerminalForTyping,
  readTerminalBuffer,
  switchToTerminalPanel,
  waitForShellReady,
} from "../terminal/mobile-terminal-helpers";
import { SessionPage } from "../../pages/session-page";
import {
  failNextWorkspaceRestore,
  restartAndAssertColdWorkspace,
  RETAINED_WORKSPACE_CONTENT,
  RETAINED_WORKSPACE_FILE,
  seedCompletedConversation,
} from "./completed-workspace-restoration-helpers";

test.describe("Completed workspace restoration on mobile", () => {
  test.describe.configure({ retries: 1 });

  test("restores Files, Changes, and Terminal after a cold restart", async ({
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
      `Mobile completed workspace restoration ${Date.now()}`,
    );
    if (!task.session_id) throw new Error("completed task has no session_id");

    const beforeSessions = await apiClient.listTaskSessions(task.id);
    const beforeMessages = await apiClient.listSessionMessages(task.session_id);
    await restartAndAssertColdWorkspace(backend, apiClient, task.id, task.session_id);

    const failure = await failNextWorkspaceRestore(
      testPage,
      task.id,
      task.session_id,
      "workspace restore failed for mobile e2e",
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 30_000 });

    await testPage.getByRole("button", { name: "Files" }).tap();
    const workspaceUnavailable = testPage.getByTestId("workspace-unavailable");
    await expect(workspaceUnavailable).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("file-tree-waiting")).toHaveCount(0);
    await expect(session.recoveryError()).toHaveCount(0);
    expect(failure.wasConsumed()).toBe(true);

    await workspaceUnavailable.getByText("Technical details", { exact: true }).tap();
    await expect(workspaceUnavailable.locator("pre")).toContainText(
      "workspace restore failed for mobile e2e",
    );

    const retry = workspaceUnavailable.getByTestId("workspace-retry");
    const retryBox = await retry.boundingBox();
    expect(retryBox, "mobile workspace retry has no rendered hitbox").not.toBeNull();
    expect(retryBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    await retry.tap();

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
    await expect(viewer).toHaveCount(0);

    await testPage.getByRole("button", { name: "Changes" }).tap();
    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible();
    await expect(changes.getByTestId("workspace-unavailable")).toHaveCount(0);

    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage, 60_000);
    await focusTerminalForTyping(testPage);
    await testPage.keyboard.type("printf 'MOBILE_RESTORED_WORKSPACE_SHELL\\n'");
    await testPage.keyboard.press("Enter");
    await expect
      .poll(() => readTerminalBuffer(testPage), {
        timeout: 15_000,
        message: "Waiting for the restored mobile shell command",
      })
      .toContain("MOBILE_RESTORED_WORKSPACE_SHELL");

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

    await prCapture.screenshot("completed-workspace-restoration-mobile", {
      caption: "Mobile completed task with restored workspace controls",
    });

    await testPage.reload();
    await testPage.getByRole("button", { name: "Chat" }).tap();
    await session.waitForLoad();
    await testPage.getByRole("button", { name: "Files" }).tap();
    await expect(session.fileTreeNode(RETAINED_WORKSPACE_FILE)).toBeVisible({ timeout: 60_000 });
    await expect(testPage.getByTestId("workspace-unavailable")).toHaveCount(0);

    const finalSessions = await apiClient.listTaskSessions(task.id);
    const finalMessages = await apiClient.listSessionMessages(task.session_id);
    expect(finalSessions.sessions).toHaveLength(beforeSessions.sessions.length);
    expect(finalMessages.messages).toHaveLength(beforeMessages.messages.length);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile completed workspace restoration");
  });
});
