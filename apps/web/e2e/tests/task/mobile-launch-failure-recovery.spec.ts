// Filename starts with "mobile-" so this runs in the mobile-chrome project.
import {
  pointSeedRepositoryAtFailingOrigin,
  pointSeedRepositoryAtUnresolvedOrigin,
  resetSeedRepositoryCheckout,
  restoreSeedRepositoryOrigin,
  test,
  expect,
} from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

async function waitForLaunchError(apiClient: ApiClient, workspaceId: string, taskId: string) {
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(workspaceId);
        const error = tasks.find((candidate) => candidate.id === taskId)?.status_summary
          ?.active_error;
        return Boolean(error?.stamp && error.category === "base_branch_missing");
      },
      { timeout: 60_000, message: "waiting for the mobile launch-error projection" },
    )
    .toBe(true);
}

test.describe("mobile task launch failure recovery", () => {
  test("uses the phone branch sheet and keeps recovery controls reachable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150_000);
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Mobile missing base recovery ${Date.now()}`,
    );
    const waiting = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(review.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    await apiClient.updateRepository(seedData.repositoryId, {
      default_branch: "mobile-default-branch-that-no-longer-exists",
      pull_before_worktree: false,
    });
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile missing base branch recovery fixture",
      {
        description: "/e2e:simple-message",
        workflow_id: workflow.id,
        workflow_step_id: waiting.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "mobile-branch-that-no-longer-exists",
          },
        ],
      },
    );
    const storedTask = await apiClient.getTask(task.id);
    const taskRepository = storedTask.repositories?.[0];
    if (!taskRepository) throw new Error("mobile fixture did not create a task repository row");

    pointSeedRepositoryAtUnresolvedOrigin(seedData, backend.tmpDir);

    try {
      await apiClient.moveTask(task.id, workflow.id, review.id);
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await waitForLaunchError(apiClient, seedData.workspaceId, task.id);

      const card = testPage.getByTestId("task-launch-error-entry");
      await expect(card).toHaveCount(1, { timeout: 30_000 });
      await expect(testPage.getByTestId("last-agent-error-notice")).toHaveCount(0);
      await expect(testPage.getByTestId("prepare-progress-panel")).toHaveCount(0);
      await expect(testPage.getByTestId("missing-branch-recovery")).toHaveCount(0);
      await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
      const actionButtons = card.locator("button[data-testid^='task-launch-']");
      await expect(actionButtons).not.toHaveCount(0);
      for (const button of await actionButtons.all()) {
        await expect(button).toBeVisible();
        await expect(button).toBeInViewport();
        const box = await button.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }

      restoreSeedRepositoryOrigin(seedData);
      await testPage.reload();
      await session.waitForLoad();
      await testPage.getByTestId("task-launch-pick_base_branch-button").tap();
      await expect(testPage.getByTestId("task-launch-branch-picker-mobile")).toBeVisible({
        timeout: 30_000,
      });
      const pickerScroll = testPage.getByTestId("task-launch-branch-picker-scroll");
      await expect(pickerScroll).toBeVisible();
      await expect
        .poll(async () => pickerScroll.evaluate((node) => getComputedStyle(node).overflowY))
        .toBe("auto");

      const branchOption = testPage.getByTestId("task-launch-branch-picker-option-main");
      await expect(branchOption).toBeVisible({ timeout: 30_000 });
      await expect(branchOption).toBeInViewport();
      await expect(pickerScroll.getByRole("option").first()).toHaveAttribute(
        "data-testid",
        "task-launch-branch-picker-option-main",
      );
      const optionBox = await branchOption.boundingBox();
      expect(optionBox).not.toBeNull();
      expect(optionBox!.height).toBeGreaterThanOrEqual(44);
      await branchOption.tap();
      await expect(testPage.getByTestId("task-launch-branch-picker-mobile")).toHaveCount(0);

      await expect
        .poll(async () => (await apiClient.getTask(task.id)).repositories?.[0]?.base_branch, {
          timeout: 60_000,
          message: "waiting for mobile row-scoped recovery",
        })
        .toBe("main");
      await expect
        .poll(
          async () => {
            const { tasks } = await apiClient.listTasks(seedData.workspaceId);
            return (
              tasks.find((candidate) => candidate.id === task.id)?.status_summary?.active_error ??
              null
            );
          },
          { timeout: 60_000, message: "waiting for the mobile launch error to clear" },
        )
        .toBeNull();

      expect(taskRepository.id).toBe((await apiClient.getTask(task.id)).repositories?.[0]?.id);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile launch recovery");
      await testPage.screenshot({
        path: testInfo.outputPath("missing-base-recovery-mobile.png"),
        fullPage: true,
      });
    } finally {
      restoreSeedRepositoryOrigin(seedData);
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      await apiClient.updateRepository(seedData.repositoryId, {
        default_branch: "main",
        pull_before_worktree: true,
      });
    }
  });

  test("starts from the local base when origin refresh fails on the phone", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150_000);
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile local base refresh",
    );
    const waiting = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(review.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile local base refresh fallback fixture",
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
          { timeout: 60_000, message: "waiting for the mobile local-base session to launch" },
        )
        .toBe(true);

      await expect
        .poll(
          async () => {
            const { tasks } = await apiClient.listTasks(seedData.workspaceId);
            return (
              tasks.find((candidate) => candidate.id === task.id)?.status_summary?.active_error ??
              null
            );
          },
          {
            timeout: 30_000,
            message: "waiting for the mobile local-base launch error to remain clear",
          },
        )
        .toBeNull();
      await expect(testPage.getByTestId("task-launch-error-entry")).toHaveCount(0);

      await assertNoDocumentHorizontalOverflow(testPage, "mobile local-base recovery");
      await testPage.screenshot({
        path: testInfo.outputPath("local-base-refresh-mobile.png"),
        fullPage: true,
      });
    } finally {
      restoreSeedRepositoryOrigin(seedData);
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
    }
  });

  test("renders the bootstrap recovery card with stacked touch controls", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Mobile bootstrap recovery presentation ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("mobile bootstrap recovery fixture has no session");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "Waiting for mobile bootstrap recovery fixture to settle",
    );

    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: "The agent could not start.",
          occurred_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
          agent_execution_id: "mobile-bootstrap-execution-e2e",
          execution_id: "mobile-bootstrap-execution-e2e",
          phase: "bootstrap",
          attempt_id: "mobile-bootstrap-execution-e2e",
          code: "generic_launch_failure",
          details: "agent_bootstrap; cause=permission_denied",
          stamp: "mobile-bootstrap-presentation-e2e",
          causes: [
            {
              operation: "resume",
              code: "permission_denied",
              detail: "The required contribution access was denied.",
            },
          ],
        },
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const card = testPage.getByTestId("session-bootstrap-recovery-card");
    await expect(card).toHaveCount(1, { timeout: 30_000 });
    await expect(testPage.getByTestId("task-launch-error-entry")).toHaveCount(0);
    await expect(testPage.getByTestId("session-recovery-error")).toHaveCount(0);

    for (const testId of [
      "recovery-resume-button",
      "recovery-restore-workspace-button",
      "recovery-fresh-button",
    ]) {
      const button = card.getByTestId(testId);
      await expect(button).toBeVisible();
      await expect(button).toBeInViewport();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    const details = card.getByTestId("session-bootstrap-recovery-details");
    await expect(details).not.toHaveAttribute("open");
    await details.getByText("Recovery details").tap();
    await expect(details).toHaveAttribute("open", "");
    await expect(card).toContainText("The required contribution access was denied.");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile bootstrap recovery presentation");

    await testPage.screenshot({
      path: testInfo.outputPath("bootstrap-recovery-presentation-mobile.png"),
      fullPage: true,
    });
    await prCapture.screenshot("bootstrap-recovery-card-mobile", {
      caption: "Mobile bootstrap recovery card with stacked touch-sized actions.",
      fullPage: true,
    });
  });
});
