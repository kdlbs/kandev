import { expect, test } from "../../fixtures/test-base";
import {
  associateNavigationPR,
  holdAgentListRead,
  holdIntegrationHealthRead,
  holdTaskEnrichmentRead,
  holdPRFeedbackRead,
  createNavigationTask,
  NAVIGATION_PR_OWNER,
  NAVIGATION_PR_REPO,
} from "../../helpers/task-navigation-efficiency";
import { SessionPage } from "../../pages/session-page";

test.describe("progressive task navigation", () => {
  test("reveals the selected desktop task while optional session enrichment is held", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const first = await createNavigationTask(
      apiClient,
      seedData,
      `Navigation anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `Navigation destination ${Date.now()}`,
    );
    if (!first.session_id || !destination.session_id)
      throw new Error("Navigation tasks must each have a session");

    const session = new SessionPage(testPage);
    let heldAgentRead: Awaited<ReturnType<typeof holdAgentListRead>> | undefined;
    let heldRead: Awaited<ReturnType<typeof holdTaskEnrichmentRead>> | undefined;
    try {
      heldAgentRead = await holdAgentListRead(testPage);
      await testPage.goto(`/t/${first.id}`);
      await session.waitForLoad();
      await heldAgentRead.requestStarted;

      heldRead = await holdTaskEnrichmentRead(testPage, destination.session_id);
      await session.sidebarTaskItem(destination.title).click();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldRead.requestStarted;

      await expect(testPage.locator('[aria-current="page"]')).toHaveText(destination.title);
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldAgentRead.requestCount()).toBe(1);
      heldRead.release();
      heldAgentRead.release();
      await session.waitForChatIdle();
      await expect(
        session.activeChat().getByRole("button", { name: "Session model settings" }),
      ).toContainText("Mock Fast");
    } finally {
      await heldAgentRead?.dispose();
      await heldRead?.dispose();
      for (const task of [first, destination]) {
        if (task.session_id) {
          await apiClient
            .stopSession({
              session_id: task.session_id,
              reason: "task navigation efficiency E2E cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });

  test("keeps the destination available while PR feedback is held and refreshes its checks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const anchor = await createNavigationTask(
      apiClient,
      seedData,
      `PR navigation anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `PR navigation target ${Date.now()}`,
    );
    if (!anchor.session_id || !destination.session_id) {
      throw new Error("PR navigation tasks must each have a session");
    }
    const prNumber = 701;
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await associateNavigationPR(apiClient, seedData.workspaceId, destination.id, prNumber);
    const heldFeedback = await holdPRFeedbackRead(
      testPage,
      seedData.workspaceId,
      NAVIGATION_PR_OWNER,
      NAVIGATION_PR_REPO,
      prNumber,
    );
    const session = new SessionPage(testPage);
    try {
      await testPage.goto(`/t/${anchor.id}`);
      await session.waitForLoad();
      await session.waitForChatIdle();

      await session.sidebarTaskItem(destination.title).click();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldFeedback.requestStarted;
      await expect(testPage.locator('[aria-current="page"]')).toHaveText(destination.title);
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldFeedback.requestCount()).toBe(1);

      heldFeedback.release();
      await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
      await session.hoverPRTopbar();
      await expect(session.prCheckGroupCount("failed")).toHaveText("1");

      await apiClient.mockGitHubSeedPRFeedback({
        owner: NAVIGATION_PR_OWNER,
        repo: NAVIGATION_PR_REPO,
        pr_number: prNumber,
        checks: [{ name: "task-switch-ci", status: "completed", conclusion: "success" }],
      });
      await apiClient.mockGitHubAssociateTaskPR({
        workspace_id: seedData.workspaceId,
        task_id: destination.id,
        owner: NAVIGATION_PR_OWNER,
        repo: NAVIGATION_PR_REPO,
        pr_number: prNumber,
        pr_url: `https://github.com/${NAVIGATION_PR_OWNER}/${NAVIGATION_PR_REPO}/pull/${prNumber}`,
        pr_title: `Task switch PR ${prNumber}`,
        head_branch: `feature/task-switch-${prNumber}`,
        base_branch: "main",
        author_login: "test-user",
        state: "open",
        checks_state: "success",
        checks_total: 1,
        checks_passing: 1,
      });
      await expect(session.prCheckGroupCount("passed")).toHaveText("1");
      await expect(session.prCheckGroup("failed")).toHaveCount(0);
    } finally {
      await heldFeedback.dispose();
      for (const task of [anchor, destination]) {
        if (task.session_id) {
          await apiClient
            .stopSession({
              session_id: task.session_id,
              reason: "PR navigation cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });

  test("shares unconfigured integration health across desktop task consumers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const anchor = await createNavigationTask(
      apiClient,
      seedData,
      `Integration health anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `PROJ-88 Integration health destination ${Date.now()}`,
    );
    if (!anchor.session_id || !destination.session_id) {
      throw new Error("Integration health tasks must each have a session");
    }
    const heldReads = await Promise.all(
      ["/api/v1/jira/config", "/api/v1/linear/config", "/api/v1/sentry/instances"].map((path) =>
        holdIntegrationHealthRead(testPage, path, seedData.workspaceId),
      ),
    );
    let heldSession: Awaited<ReturnType<typeof holdTaskEnrichmentRead>> | undefined;
    const session = new SessionPage(testPage);
    try {
      await testPage.goto(`/t/${anchor.id}`);
      await session.waitForLoad();
      await Promise.all(heldReads.map((read) => read.requestStarted));

      heldSession = await holdTaskEnrichmentRead(testPage, destination.session_id);
      await session.sidebarTaskItem(destination.title).click();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldSession.requestStarted;
      await expect(testPage.locator('[aria-current="page"]')).toHaveText(destination.title);
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldReads.map((read) => read.requestCount())).toEqual([1, 1, 1]);

      heldSession.release();
      heldReads.forEach((read) => read.release());
      await session.waitForChatIdle();
      await expect(testPage.getByRole("button", { name: "PROJ-88", exact: true })).toHaveCount(0);
      expect(heldReads.map((read) => read.requestCount())).toEqual([1, 1, 1]);
    } finally {
      await heldSession?.dispose();
      await Promise.all(heldReads.map((read) => read.dispose()));
      for (const task of [anchor, destination]) {
        if (task.session_id) {
          await apiClient
            .stopSession({
              session_id: task.session_id,
              reason: "integration health navigation cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });
});
