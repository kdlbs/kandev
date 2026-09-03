import {
  pointSeedRepositoryAtFailingOrigin,
  pointSeedRepositoryAtUnresolvedOrigin,
  restoreSeedRepositoryOrigin,
  test,
  expect,
} from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

type LaunchError = {
  session_id?: string;
  task_repository_id?: string;
  stamp?: string;
  category?: string;
  recovery_actions?: string[];
  preview?: string;
};

async function taskLaunchError(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<LaunchError | null> {
  const { tasks } = await apiClient.listTasks(workspaceId);
  const task = tasks.find((candidate: { id: string }) => candidate.id === taskId);
  return (task?.status_summary?.active_error as LaunchError | null | undefined) ?? null;
}

async function waitForTaskLaunchError(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  category?: string,
): Promise<LaunchError> {
  await expect
    .poll(
      async () => {
        const error = await taskLaunchError(apiClient, workspaceId, taskId);
        return Boolean(error?.stamp && (!category || error.category === category));
      },
      { timeout: 60_000, message: `waiting for typed launch error on ${taskId}` },
    )
    .toBe(true);
  const error = await taskLaunchError(apiClient, workspaceId, taskId);
  if (!error?.stamp) throw new Error(`task ${taskId} has no typed launch error`);
  return error;
}

async function waitForLaunchErrorCleared(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<void> {
  await expect
    .poll(async () => (await taskLaunchError(apiClient, workspaceId, taskId)) === null, {
      timeout: 60_000,
      message: `waiting for launch error ${taskId} to clear`,
    })
    .toBe(true);
}

async function recoveryWorkflow(apiClient: ApiClient, workspaceId: string, name: string) {
  const workflow = await apiClient.createWorkflow(workspaceId, name);
  const waiting = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
  const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
  await apiClient.updateWorkflowStep(review.id, {
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  return { workflow, waiting, review, done };
}

test.describe("task launch failure recovery", () => {
  test("gates a closed PR before session creation and persists Mark review done", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);

    const { workflow, waiting, review, done } = await recoveryWorkflow(
      apiClient,
      seedData.workspaceId,
      `Launch recovery gate ${Date.now()}`,
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 1201,
        title: "Merged launch fixture",
        state: "closed",
        head_branch: "feature/merged-launch-fixture",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const task = await apiClient.createTask(seedData.workspaceId, "Closed PR launch fixture", {
      workflow_id: workflow.id,
      workflow_step_id: waiting.id,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      repositories: [
        {
          repository_id: seedData.repositoryId,
          base_branch: "main",
          checkout_branch: "feature/merged-launch-fixture",
          pr_number: 1201,
        },
      ],
      metadata: { agent_profile_id: seedData.agentProfileId },
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      repository_id: seedData.repositoryId,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 1201,
      pr_url: "https://github.com/testorg/testrepo/pull/1201",
      pr_title: "Merged launch fixture",
      head_branch: "feature/merged-launch-fixture",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    await apiClient.moveTask(task.id, workflow.id, review.id);
    const launchError = await waitForTaskLaunchError(
      apiClient,
      seedData.workspaceId,
      task.id,
      "pr_already_closed",
    );
    expect(launchError.recovery_actions).toEqual(["mark_review_done"]);

    const sessionsBeforeOpen = await apiClient.listTaskSessions(task.id);
    expect(sessionsBeforeOpen.sessions).toHaveLength(0);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const card = testPage.getByTestId("task-launch-error-entry");
    await expect(card).toHaveCount(1, { timeout: 30_000 });
    await expect(card).toContainText("The task is linked to a closed pull request.");

    await testPage.reload();
    await session.waitForLoad();
    await expect(testPage.getByTestId("task-launch-error-entry")).toHaveCount(1);
    await testPage.getByTestId("task-launch-mark_review_done-button").click();

    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
        timeout: 30_000,
        message: "waiting for the task to move to Done",
      })
      .toBe(done.id);
    await waitForLaunchErrorCleared(apiClient, seedData.workspaceId, task.id);
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);

    await testPage.screenshot({
      path: testInfo.outputPath("closed-pr-launch-gate-desktop.png"),
      fullPage: true,
    });
  });

  test("recovers an exact task-repository row and persists the self-heal", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150_000);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Missing base recovery ${Date.now()}`,
    );
    const waiting = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(review.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    await apiClient.updateRepository(seedData.repositoryId, {
      default_branch: "default-branch-that-no-longer-exists",
      pull_before_worktree: false,
    });
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Missing base branch recovery fixture",
      {
        description: "/e2e:simple-message",
        workflow_id: workflow.id,
        workflow_step_id: waiting.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "branch-that-no-longer-exists",
          },
        ],
      },
    );
    const storedTask = await apiClient.getTask(task.id);
    const taskRepository = storedTask.repositories?.[0];
    if (!taskRepository) throw new Error("launch fixture did not create a task repository row");

    pointSeedRepositoryAtUnresolvedOrigin(seedData, backend.tmpDir);

    try {
      await apiClient.moveTask(task.id, workflow.id, review.id);
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const launchError = await waitForTaskLaunchError(apiClient, seedData.workspaceId, task.id);
      expect(launchError.task_repository_id).toBe(taskRepository.id);
      expect(launchError.recovery_actions).toEqual(["retry_default", "pick_base_branch"]);

      const pointerToast = testPage
        .getByTestId("toast-message")
        .filter({ hasText: "The task launch failed. Open the task details for recovery actions." });
      await expect(pointerToast).toBeVisible({ timeout: 30_000 });
      await expect(pointerToast).not.toContainText("branch-that-no-longer-exists");

      const card = testPage.getByTestId("task-launch-error-entry");
      await expect(card).toHaveCount(1, { timeout: 30_000 });
      await expect(card).toContainText("The selected base branch is not available.");
      await expect(card).not.toContainText("branch-that-no-longer-exists");

      restoreSeedRepositoryOrigin(seedData);
      await testPage.reload();
      await session.waitForLoad();
      await expect(testPage.getByTestId("task-launch-error-entry")).toHaveCount(1);

      await testPage.getByTestId("task-launch-pick_base_branch-button").click();
      await expect(testPage.getByTestId("task-launch-branch-picker-option-main")).toBeVisible({
        timeout: 30_000,
      });
      await testPage.getByTestId("task-launch-branch-picker-option-main").click();
      await expect(testPage.getByTestId("task-launch-branch-picker-option-main")).toHaveCount(0);

      await expect
        .poll(async () => (await apiClient.getTask(task.id)).repositories?.[0]?.base_branch, {
          timeout: 60_000,
          message: "waiting for the exact task repository base to self-heal",
        })
        .toBe("main");
      await waitForLaunchErrorCleared(apiClient, seedData.workspaceId, task.id);

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return sessions.some((item) =>
              ["RUNNING", "WAITING_FOR_INPUT", "IDLE", "COMPLETED"].includes(item.state),
            );
          },
          { timeout: 60_000, message: "waiting for the recovered session to launch" },
        )
        .toBe(true);

      await expect(
        testPage.getByTestId("toast-message").filter({ hasText: "Recovery could not start" }),
      ).toHaveCount(0);

      await assertNoDocumentHorizontalOverflow(testPage, "desktop launch recovery");
      await testPage.screenshot({
        path: testInfo.outputPath("missing-base-recovery-desktop.png"),
        fullPage: true,
      });
    } finally {
      restoreSeedRepositoryOrigin(seedData);
      await apiClient.updateRepository(seedData.repositoryId, {
        default_branch: "main",
        pull_before_worktree: true,
      });
    }
  });

  test("starts from the local base when origin refresh fails", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150_000);

    const { workflow, waiting, review } = await recoveryWorkflow(
      apiClient,
      seedData.workspaceId,
      "Local base refresh",
    );
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Local base refresh fallback fixture",
      {
        description: "/e2e:simple-message",
        workflow_id: workflow.id,
        workflow_step_id: waiting.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
      },
    );

    pointSeedRepositoryAtFailingOrigin(seedData, backend.tmpDir);
    try {
      await apiClient.moveTask(task.id, workflow.id, review.id);
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return sessions.some((item) =>
              ["RUNNING", "WAITING_FOR_INPUT", "IDLE", "COMPLETED"].includes(item.state),
            );
          },
          { timeout: 60_000, message: "waiting for the local-base session to launch" },
        )
        .toBe(true);

      await expect
        .poll(() => taskLaunchError(apiClient, seedData.workspaceId, task.id), {
          timeout: 30_000,
          message: "waiting for the local-base launch error to remain clear",
        })
        .toBeNull();
      await expect(testPage.getByTestId("task-launch-error-entry")).toHaveCount(0);

      await assertNoDocumentHorizontalOverflow(testPage, "desktop local-base recovery");
      await testPage.screenshot({
        path: testInfo.outputPath("local-base-refresh-desktop.png"),
        fullPage: true,
      });
    } finally {
      restoreSeedRepositoryOrigin(seedData);
    }
  });
});
