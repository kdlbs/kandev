import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  associateNavigationPR,
  holdAgentListRead,
  holdIntegrationHealthRead,
  holdPRFeedbackRead,
  holdTaskEnrichmentRead,
  createNavigationTask,
  NAVIGATION_PR_OWNER,
  NAVIGATION_PR_REPO,
} from "../../helpers/task-navigation-efficiency";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile progressive task navigation", () => {
  test("reveals the selected task while optional session enrichment is held", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const first = await createNavigationTask(
      apiClient,
      seedData,
      `Mobile navigation anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `Mobile navigation destination ${Date.now()}`,
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
      await testPage.getByTestId("mobile-task-picker-trigger").tap();
      const taskSheet = testPage.getByRole("dialog", { name: "Tasks", exact: true });
      await taskSheet.getByTestId("sidebar-task-item").filter({ hasText: destination.title }).tap();
      await expect(taskSheet).not.toBeVisible();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldRead.requestStarted;

      await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(
        destination.title,
      );
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldAgentRead.requestCount()).toBe(1);
      await heldRead.release();
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
              reason: "mobile task navigation efficiency E2E cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });

  test("keeps the destination available while mobile PR feedback is held and refreshes checks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const anchor = await createNavigationTask(
      apiClient,
      seedData,
      `Mobile PR anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `Mobile PR destination ${Date.now()}`,
    );
    if (!anchor.session_id || !destination.session_id) {
      throw new Error("Mobile PR navigation tasks must each have a session");
    }
    const prNumber = 702;
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

      await testPage.getByTestId("mobile-task-picker-trigger").tap();
      const taskSheet = testPage.getByRole("dialog", { name: "Tasks", exact: true });
      await taskSheet.getByTestId("sidebar-task-item").filter({ hasText: destination.title }).tap();
      await expect(taskSheet).not.toBeVisible();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldFeedback.requestStarted;
      await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(
        destination.title,
      );
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldFeedback.requestCount()).toBe(1);

      heldFeedback.release();
      await expect(session.prStatusChip()).toBeVisible({ timeout: 15_000 });
      await session.tapPRStatusChip();
      const failed = session
        .prStatusChipDrawer()
        .locator("[data-testid='pr-check-group'][data-kind='failed']");
      await expect(failed.getByTestId("pr-check-group-count")).toHaveText("1");

      await waitForFiniteAnimations(session.prStatusChipDrawer());
      await session.prStatusChipDrawerClose().tap();
      await expect(session.prStatusChipDrawer()).toHaveCount(0);
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
      await session.tapPRStatusChip();
      const passed = session
        .prStatusChipDrawer()
        .locator("[data-testid='pr-check-group'][data-kind='passed']");
      await expect(passed.getByTestId("pr-check-group-count")).toHaveText("1");
    } finally {
      await heldFeedback.dispose();
      for (const task of [anchor, destination]) {
        if (task.session_id) {
          await apiClient
            .stopSession({
              session_id: task.session_id,
              reason: "mobile PR navigation cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });

  test("shares unconfigured integration health across mobile task consumers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const anchor = await createNavigationTask(
      apiClient,
      seedData,
      `Mobile integration health anchor ${Date.now()}`,
    );
    const destination = await createNavigationTask(
      apiClient,
      seedData,
      `PROJ-89 Mobile integration health destination ${Date.now()}`,
    );
    if (!anchor.session_id || !destination.session_id) {
      throw new Error("Mobile integration health tasks must each have a session");
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
      await testPage.getByTestId("mobile-task-picker-trigger").tap();
      const taskSheet = testPage.getByRole("dialog", { name: "Tasks", exact: true });
      await taskSheet.getByTestId("sidebar-task-item").filter({ hasText: destination.title }).tap();
      await expect(taskSheet).not.toBeVisible();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
      await heldSession.requestStarted;
      await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(
        destination.title,
      );
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      expect(heldReads.map((read) => read.requestCount())).toEqual([1, 1, 1]);

      heldSession.release();
      heldReads.forEach((read) => read.release());
      await session.waitForChatIdle();
      await expect(testPage.getByRole("button", { name: "PROJ-89", exact: true })).toHaveCount(0);
      expect(heldReads.map((read) => read.requestCount())).toEqual([1, 1, 1]);
    } finally {
      await heldSession?.dispose();
      await Promise.all(heldReads.map((read) => read.dispose()));
      for (const task of [anchor, destination]) {
        if (task.session_id) {
          await apiClient
            .stopSession({
              session_id: task.session_id,
              reason: "mobile integration health navigation cleanup",
              force: true,
            })
            .catch(() => undefined);
        }
        await apiClient.deleteTask(task.id).catch(() => undefined);
      }
    }
  });
});
