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

async function openMobilePanel(page: import("@playwright/test").Page, name: RegExp) {
  await page.getByRole("navigation").getByRole("button", { name }).tap();
}

test.describe("Mobile live repository attachment", () => {
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
      "Mobile live repository attachment",
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

    await openMobilePanel(testPage, /Changes$/);
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("primary-change.txt", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const readyEvent = watcher.waitForEvent("session.agentctl_ready", {
      timeout: 90_000,
      where: (payload) =>
        payload.session_id === task.session_id &&
        typeof payload.task_workspace_path === "string" &&
        payload.task_workspace_path !== "",
    });
    await apiClient.addUserMessage(
      task.id,
      task.session_id,
      liveAttachmentPrompt(repository.repositoryPath),
    );
    await readyEvent;
    const promoted = await waitForPromotedSession(apiClient, task.id, task.session_id, primaryPath);
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
    await openMobilePanel(testPage, /Files$/);
    await openMobilePanel(testPage, /Changes$/);
    await postSecondaryRefresh;

    await expect(testPage.getByTestId("changes-repo-group")).toHaveCount(2, { timeout: 30_000 });
    await expect(
      testPage.getByTestId("changes-repo-header").filter({ hasText: "secondary-repository" }),
    ).toBeVisible();
    await expect(testPage.getByText("primary-change.txt", { exact: true })).toBeVisible();
    await expect(testPage.getByText("secondary-change.txt", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("changes-refresh-status")).toHaveCount(0);
    await prCapture.screenshot("mobile-changes-reconciled-repositories", {
      caption:
        "Phone Changes with both current repositories after live attachment and no refresh warning",
    });

    await openMobilePanel(testPage, /Chat$/);
    const chat = session.activeChat();
    await chat.getByRole("link", { name: "primary after" }).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(viewer.locator(".cm-content")).toContainText(LIVE_ATTACHMENT_WORK_ORDER.trim());
    await prCapture.screenshot("mobile-chat-link-primary-checkout", {
      caption: "Phone file viewer opening the work order from the agent's original checkout",
    });
    await viewer.getByRole("button", { name: /Close/i }).tap();
    await openMobilePanel(testPage, /Chat$/);

    await session.activeChat().getByRole("link", { name: "secondary README" }).tap();
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(viewer.locator(".cm-content")).toContainText("secondary checkout readme");
    await viewer.getByRole("button", { name: /Close/i }).tap();
    await openMobilePanel(testPage, /Chat$/);

    const postReloadRefresh = watcher.waitForResponse("session.git.refresh", {
      timeout: 90_000,
      requestWhere: (payload) => payload.session_id === task.session_id,
      where: (payload) => {
        const snapshots = (payload as { snapshots?: unknown[] }).snapshots;
        return Array.isArray(snapshots) && snapshots.length >= 2;
      },
    });
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    const reloadedChat = session.activeChat();
    await reloadedChat.getByRole("link", { name: "primary after" }).tap();
    await expect(viewer.locator(".cm-content")).toContainText(LIVE_ATTACHMENT_WORK_ORDER.trim());
    await viewer.getByRole("button", { name: /Close/i }).tap();
    await openMobilePanel(testPage, /Chat$/);

    await openMobilePanel(testPage, /Changes$/);
    await expect(testPage.getByTestId("changes-repo-group")).toHaveCount(2, { timeout: 30_000 });
    await expect(testPage.getByTestId("changes-refresh-status")).toHaveCount(0);
    await expect(testPage.getByText("primary-change.txt", { exact: true })).toBeVisible();
    await expect(testPage.getByText("secondary-change.txt", { exact: true })).toBeVisible();
    await postReloadRefresh;
  });
});
