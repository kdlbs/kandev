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
import { readTranscriptScrollState, setTranscriptScrollTop } from "../../helpers/transcript-scroll";
import { SessionPage } from "../../pages/session-page";

async function taskLaunchError(apiClient: ApiClient, workspaceId: string, taskId: string) {
  const { tasks } = await apiClient.listTasks(workspaceId);
  const summary = tasks.find((candidate) => candidate.id === taskId)?.status_summary;
  const legacy = summary?.active_error;
  return (
    summary?.task_error ??
    (legacy && !legacy.session_id && legacy.scope !== "session" ? legacy : null)
  );
}

async function waitForLaunchError(apiClient: ApiClient, workspaceId: string, taskId: string) {
  await expect
    .poll(
      async () => {
        const error = await taskLaunchError(apiClient, workspaceId, taskId);
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

      const sharedError = testPage.getByTestId("task-shared-error");
      await expect(sharedError).toBeVisible({ timeout: 30_000 });
      await testPage.getByTestId("task-shared-error-details").tap();
      const card = testPage.getByTestId("task-launch-error-entry");
      await expect(card).toHaveCount(1);
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
      await expect(testPage.getByTestId("task-shared-error")).toBeVisible();
      await testPage.getByTestId("task-shared-error-details").tap();
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
        .poll(() => taskLaunchError(apiClient, seedData.workspaceId, task.id), {
          timeout: 60_000,
          message: "waiting for the mobile launch error to clear",
        })
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
        .poll(() => taskLaunchError(apiClient, seedData.workspaceId, task.id), {
          timeout: 30_000,
          message: "waiting for the mobile local-base launch error to remain clear",
        })
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

  test("retains session failure history with stacked touch controls", async ({
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
    await apiClient.seedAgentMessages(task.session_id, 40, "mobile bootstrap history");

    const failureStamp = "mobile-bootstrap-presentation-e2e";
    const failureCreatedAt = new Date().toISOString();
    const longDetails = Array.from(
      { length: 12 },
      (_, index) => `agent_bootstrap; diagnostic_line=${index + 1}; cause=permission_denied`,
    ).join("\n");
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: "The agent could not start.",
          occurred_at: failureCreatedAt,
          agent_execution_id: "mobile-bootstrap-execution-e2e",
          execution_id: "mobile-bootstrap-execution-e2e",
          phase: "bootstrap",
          attempt_id: "mobile-bootstrap-execution-e2e",
          code: "generic_launch_failure",
          details: longDetails,
          scope: "session",
          stamp: failureStamp,
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
    await apiClient.seedSessionMessage(task.session_id, {
      type: "status",
      content: "Agent encountered an error: The agent could not start.",
      createdAt: failureCreatedAt,
      metadata: {
        recovery_actions: true,
        scope: "session",
        error_stamp: failureStamp,
        error_output: longDetails,
        actions: [
          {
            type: "ws_request",
            label: "Resume session",
            icon: "refresh",
            test_id: "recovery-resume-button",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: task.session_id, action: "resume" },
            },
          },
          {
            type: "ws_request",
            label: "Start fresh session",
            icon: "player-play",
            test_id: "recovery-fresh-button",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: task.session_id, action: "fresh_start" },
            },
          },
        ],
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const failureText = "Agent encountered an error: The agent could not start.";
    const failureRows = testPage.locator("[id^='msg-']").filter({ hasText: failureText });
    await expect(failureRows).toHaveCount(1, { timeout: 30_000 });
    const failureRow = failureRows.first();
    await expect(testPage.getByTestId("task-shared-error")).toHaveCount(0);
    const transcript = session.activeChat().locator(".chat-message-list").first();
    await expect
      .poll(async () => (await readTranscriptScrollState(transcript)).scrollOwnerCount)
      .toBe(1);
    const initialScroll = await readTranscriptScrollState(transcript);
    expect(initialScroll.scrollHeight).toBeGreaterThan(initialScroll.clientHeight);
    expect(initialScroll.scrollTop).toBeGreaterThan(0);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();

    for (const testId of ["recovery-resume-button", "recovery-fresh-button"]) {
      const button = failureRow.getByTestId(testId);
      await expect(button).toBeVisible();
      await expect(button).toBeInViewport();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    const details = failureRow.getByText("Technical details", { exact: true });
    await details.tap();
    const detailsPanel = failureRow.locator("details");
    await expect(detailsPanel).toHaveAttribute("open", "");
    await expect(detailsPanel).toContainText(
      "agent_bootstrap; diagnostic_line=1; cause=permission_denied",
    );
    const expandedScroll = await readTranscriptScrollState(transcript);
    expect(expandedScroll.scrollOwnerCount).toBe(1);
    expect(expandedScroll.scrollHeight).toBeGreaterThan(expandedScroll.clientHeight);
    const middleScrollTop = await setTranscriptScrollTop(
      transcript,
      Math.floor((expandedScroll.scrollHeight - expandedScroll.clientHeight) / 2),
    );
    expect(middleScrollTop).toBeGreaterThan(0);

    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: "The agent could not start.",
          occurred_at: failureCreatedAt,
          agent_execution_id: "mobile-bootstrap-execution-e2e",
          execution_id: "mobile-bootstrap-execution-e2e",
          phase: "bootstrap",
          attempt_id: "mobile-bootstrap-execution-e2e",
          code: "generic_launch_failure",
          details: longDetails,
          scope: "session",
          stamp: failureStamp,
          dismissed_at: new Date(Date.now() + 1_000).toISOString(),
        },
      },
    });
    await apiClient.seedSessionMessage(task.session_id, {
      type: "script_execution",
      content: "Agent resumed",
      createdAt: new Date(Date.now() + 2_000).toISOString(),
      metadata: {
        script_type: "agent_boot",
        is_resuming: true,
        status: "exited",
        exit_code: 0,
      },
    });
    await apiClient.seedSessionMessage(task.session_id, {
      type: "message",
      content: "Recovered agent output",
      createdAt: new Date(Date.now() + 3_000).toISOString(),
    });
    await expect(failureRow).toContainText(failureText);
    await expect(failureRow.getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect
      .poll(async () => (await readTranscriptScrollState(transcript)).scrollTop)
      .toBeGreaterThanOrEqual(Math.max(0, middleScrollTop - 2));

    const nextFailureStamp = "mobile-bootstrap-presentation-e2e-new";
    const nextFailureCreatedAt = new Date(Date.now() + 4_000).toISOString();
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: "The agent could not start.",
          occurred_at: nextFailureCreatedAt,
          agent_execution_id: "mobile-bootstrap-execution-e2e-new",
          execution_id: "mobile-bootstrap-execution-e2e-new",
          phase: "bootstrap",
          attempt_id: "mobile-bootstrap-execution-e2e-new",
          code: "generic_launch_failure",
          details: longDetails,
          scope: "session",
          stamp: nextFailureStamp,
        },
      },
    });
    await apiClient.seedSessionMessage(task.session_id, {
      type: "status",
      content: failureText,
      createdAt: nextFailureCreatedAt,
      metadata: {
        recovery_actions: true,
        scope: "session",
        error_stamp: nextFailureStamp,
        error_output: longDetails,
        actions: [
          {
            type: "ws_request",
            label: "Resume session",
            icon: "refresh",
            test_id: "recovery-resume-button",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: task.session_id, action: "resume" },
            },
          },
        ],
      },
    });
    await expect(failureRows).toHaveCount(2);
    const currentFailureRow = failureRows.last();
    await expect(currentFailureRow.getByTestId("recovery-resume-button")).toBeVisible();
    await expect(failureRow.getByTestId("recovery-resume-button")).toHaveCount(0);

    await testPage.reload();
    await session.waitForLoad();
    const reloadedFailureRows = testPage.locator("[id^='msg-']").filter({ hasText: failureText });
    await expect(reloadedFailureRows).toHaveCount(2);
    await expect(reloadedFailureRows.first().getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect(reloadedFailureRows.last().getByTestId("recovery-resume-button")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile bootstrap recovery presentation");

    await testPage.screenshot({
      path: testInfo.outputPath("bootstrap-recovery-presentation-mobile.png"),
      fullPage: true,
    });
    await prCapture.screenshot("bootstrap-recovery-card-mobile", {
      caption: "Mobile transcript keeps recovered session failure history and current actions.",
      fullPage: true,
    });
  });
});
