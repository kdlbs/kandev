import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  createLiveAttachmentRepository,
  LIVE_ATTACHMENT_WORK_ORDER,
  liveAttachmentPrompt,
  primaryCheckoutPath,
  waitForPromotedSession,
  writePrimaryWorkOrder,
} from "../../helpers/live-repository-attachment";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

function activeFileEditor(page: import("@playwright/test").Page) {
  return page.locator(".monaco-editor:visible").first();
}

test.describe("Live repository attachment", () => {
  test.setTimeout(180_000);

  test("preserves chat links and Changes health across a legacy in-turn attachment", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repository = createLiveAttachmentRepository(backend.tmpDir);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Live repository attachment",
      seedData.agentProfileId,
      {
        description: "/e2e:empty-turn",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "the initial turn should finish before attachment",
      90_000,
    );
    const initialSessions = await apiClient.listTaskSessions(task.id);
    const initialSession = initialSessions.sessions.find(
      (session) => session.id === task.session_id,
    );
    if (!initialSession) throw new Error("initial session was not returned");
    const primaryPath = primaryCheckoutPath(initialSession);
    if (!primaryPath) throw new Error("initial session has no primary checkout");
    writePrimaryWorkOrder(primaryPath);

    const watcher = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });

    await session.clickTab("Changes");
    const changes = session.changes;
    await expect(changes.getByText("primary-change.txt", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const readyEvent = watcher.waitForEvent("session.agentctl_ready", {
      timeout: 90_000,
      where: (payload) =>
        payload.session_id === task.session_id &&
        typeof payload.task_workspace_path === "string" &&
        payload.task_workspace_path !== "",
    });
    const refreshedInventory = watcher.waitForResponse("session.git.refresh", {
      timeout: 90_000,
      requestWhere: (payload) => payload.session_id === task.session_id,
      where: (payload) => {
        const snapshots = (payload as { snapshots?: unknown[] }).snapshots;
        return Array.isArray(snapshots) && snapshots.length >= 2;
      },
    });

    await apiClient.addUserMessage(
      task.id,
      task.session_id,
      liveAttachmentPrompt(repository.repositoryPath),
    );
    await readyEvent;
    await refreshedInventory;
    const promoted = await waitForPromotedSession(apiClient, task.id, task.session_id, primaryPath);
    expect(promoted.workspace_path).not.toBe(primaryPath);
    const secondaryPath = (promoted.worktrees ?? [])
      .flatMap((worktree) => (worktree.worktree_path ? [worktree.worktree_path] : []))
      .find((worktree) => worktree !== primaryPath);
    if (!secondaryPath) throw new Error("attached worktree path was not returned");
    fs.writeFileSync(path.join(secondaryPath, "secondary-change.txt"), "secondary change\n");
    const postSecondaryRefresh = watcher.waitForResponse("session.git.refresh", {
      timeout: 30_000,
      requestWhere: (payload) => payload.session_id === task.session_id,
      where: (payload) => {
        const snapshots = (payload as { snapshots?: unknown[] }).snapshots;
        return Array.isArray(snapshots) && snapshots.length >= 2;
      },
    });
    await session.clickTab("Files");
    await session.clickTab("Changes");
    await postSecondaryRefresh;

    await expect(changes.getByTestId("changes-repo-group")).toHaveCount(2, { timeout: 30_000 });
    await expect(
      changes.getByTestId("changes-repo-header").filter({ hasText: "secondary-repository" }),
    ).toBeVisible();
    await expect(changes.getByText("primary-change.txt", { exact: true })).toBeVisible();
    await expect(changes.getByText("secondary-change.txt", { exact: true })).toBeVisible();
    await expect(changes.getByTestId("changes-refresh-status")).toHaveCount(0);
    await prCapture.screenshot("changes-reconciled-repositories", {
      caption:
        "Desktop Changes with both current repositories after live attachment and no refresh warning",
    });

    await session.showSessionContext();
    const chat = session.activeChat();
    await chat.getByRole("link", { name: "primary after" }).click();
    const editor = activeFileEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 15_000 });
    await expect(editor.locator(".view-lines")).toContainText(LIVE_ATTACHMENT_WORK_ORDER.trim());
    await prCapture.screenshot("chat-link-primary-checkout", {
      caption: "Desktop file panel opening the work order from the agent's original checkout",
    });

    await session.showSessionContext();
    await chat.getByRole("link", { name: "primary qualified" }).click();
    await session.clickTab("Files");
    await expect(activeFileEditor(testPage).locator(".view-lines")).toContainText(
      "primary checkout readme",
    );

    await session.showSessionContext();
    await chat.getByRole("link", { name: "secondary README" }).click();
    await session.clickTab("Files");
    await expect(activeFileEditor(testPage).locator(".view-lines")).toContainText(
      "secondary checkout readme",
    );

    const postReloadRefresh = watcher.waitForResponse("session.git.refresh", {
      timeout: 90_000,
      requestWhere: (payload) => payload.session_id === task.session_id,
      where: (payload) => {
        const snapshots = (payload as { snapshots?: unknown[] }).snapshots;
        return Array.isArray(snapshots) && snapshots.length >= 2;
      },
    });
    await testPage.reload();
    // The reload restores the Changes layout, so foreground the session tab
    // before waiting for the chat surface.
    await session.showSessionContext();
    await session.waitForChatIdle({ timeout: 60_000 });
    const reloadedChat = session.activeChat();
    await reloadedChat.getByRole("link", { name: "primary after" }).click();
    await session.clickTab("Files");
    await expect(activeFileEditor(testPage).locator(".view-lines")).toContainText(
      LIVE_ATTACHMENT_WORK_ORDER.trim(),
    );
    await session.showSessionContext();
    await reloadedChat.getByRole("link", { name: "secondary README" }).click();
    await session.clickTab("Files");
    await expect(activeFileEditor(testPage).locator(".view-lines")).toContainText(
      "secondary checkout readme",
    );

    await session.clickTab("Changes");
    await expect(session.changes.getByTestId("changes-repo-group")).toHaveCount(2, {
      timeout: 30_000,
    });
    await expect(session.changes.getByTestId("changes-refresh-status")).toHaveCount(0);
    await expect(session.changes.getByText("primary-change.txt", { exact: true })).toBeVisible();

    // The reconnect request must correlate to this session and return both
    // current repositories, so reload starts from a reconciled inventory.
    await postReloadRefresh;
  });
});
