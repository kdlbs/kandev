import { test, expect } from "../../fixtures/test-base";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import { SessionPage } from "../../pages/session-page";
import { openBlockedTaskLoadingState } from "./task-loading-state-helpers";

test.describe("Task loading state", () => {
  test("shows a spinner instead of a blank task detail pane while task data loads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const unblockTaskDetailRequest = await openBlockedTaskLoadingState({
      testPage,
      apiClient,
      seedData,
      title: "Task Loading State Anchor",
      unresolvedTaskId: "unresolved-task-detail-desktop",
    });

    try {
      await expect(testPage.getByTestId("task-loading-state")).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByText("Loading task...")).toBeVisible();
    } finally {
      await unblockTaskDetailRequest();
    }
  });

  test("keeps sibling tasks available after a missing task route", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sibling = await apiClient.createTask(seedData.workspaceId, "Missing route sibling", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/missing-task-route-desktop?workspaceId=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("task-load-error-state")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("task-unavailable-overview-link")).toBeVisible();

    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(sibling.id)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

    await sidebar.plainClick(sibling.id);
    await expect(testPage).toHaveURL(new RegExp(`/t/${sibling.id}$`));
    await expect(testPage.getByTestId("task-load-error-state")).toBeHidden({ timeout: 10_000 });
  });

  test("keeps a session-backed sibling active", async ({ testPage, apiClient, seedData }) => {
    const sibling = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Missing route session sibling",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    if (!sibling.session_id) throw new Error("session-backed sibling has no session");

    await testPage.goto(`/t/missing-task-route-session?workspaceId=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("task-load-error-state")).toBeVisible({ timeout: 10_000 });

    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(sibling.id)).toBeVisible({ timeout: 10_000 });
    await sidebar.plainClick(sibling.id);

    await expect(testPage).toHaveURL(new RegExp(`/t/${sibling.id}$`));
    await expect(testPage.getByTestId("task-load-error-state")).toBeHidden({ timeout: 10_000 });
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 10_000 });
    await expect(sidebar.row(sibling.id)).toHaveAttribute("data-active", "true", {
      timeout: 10_000,
    });
  });

  test("restores the last selected session after an A to B to A switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session route sync task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session route sync task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    if (!taskA.session_id || !taskB.session_id) {
      throw new Error("session-backed route sync tasks have no primary session");
    }

    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    let secondarySessionId: string | undefined;
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskA.id);
          secondarySessionId = sessions.find((item) => item.id !== taskA.session_id)?.id;
          return secondarySessionId ?? null;
        },
        { timeout: 30_000, message: "Waiting for task A secondary session" },
      )
      .toBeTruthy();

    await expect(session.sessionTabBySessionId(secondarySessionId!)).toBeVisible({
      timeout: 15_000,
    });
    await session.sessionTabBySessionId(secondarySessionId!).click();
    await expect
      .poll(async () =>
        testPage.evaluate(
          () =>
            (
              window as unknown as {
                __KANDEV_E2E_STORE__?: { getState: () => { tasks: { activeSessionId: string } } };
              }
            ).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId,
        ),
      )
      .toBe(secondarySessionId);

    await session.clickTaskInSidebar("Session route sync task B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}$`), { timeout: 15_000 });
    await session.waitForLoad();
    await expect(session.sessionTabBySessionId(taskB.session_id)).toBeVisible({
      timeout: 15_000,
    });

    await session.clickTaskInSidebar("Session route sync task A");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskA.id}$`), { timeout: 15_000 });
    await session.waitForLoad();
    await expect
      .poll(async () =>
        testPage.evaluate(
          () =>
            (
              window as unknown as {
                __KANDEV_E2E_STORE__?: { getState: () => { tasks: { activeSessionId: string } } };
              }
            ).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId,
        ),
      )
      .toBe(secondarySessionId);
  });
});
