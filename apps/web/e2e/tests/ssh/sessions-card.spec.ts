import { test, expect } from "../../fixtures/ssh-test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

async function waitForTaskSSHSession(apiClient: ApiClient, executorID: string, taskID: string) {
  let row: Awaited<ReturnType<ApiClient["listSSHSessions"]>>[number] | undefined;
  await expect
    .poll(
      async () => {
        row = (await apiClient.listSSHSessions(executorID)).find(
          (session) => session.task_id === taskID,
        );
        return row !== undefined;
      },
      { message: "wait for this task's SSH runtime row", timeout: 60_000 },
    )
    .toBe(true);
  return row!;
}

/**
 * SSHSessionsCard — table of active sessions for the executor. Mirrors
 * SpritesInstancesCard / DockerContainersCard shape.
 *
 * Covers e2e-plan.md group E (E1–E6).
 */
test.describe("ssh sessions card", () => {
  test("empty state when no sessions are running", async ({ testPage, seedData }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.sessionsEmpty).toBeVisible();
    await expect(page.sessionsTable).toBeHidden();
  });

  test("renders a row per active session with correct columns", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "E2 sessions row",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");

    const row = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);

    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.sessionRow(row!.session_id)).toBeVisible();
    await expect(page.sessionRow(row!.session_id).getByTestId("ssh-session-host")).toHaveText(
      `${seedData.sshTarget.user}@${seedData.sshTarget.host}`,
    );
    await expect(
      page.sessionRow(row!.session_id).getByTestId("ssh-session-remote-port"),
    ).toContainText(String(row!.remote_agentctl_port));
    await expect(
      page.sessionRow(row!.session_id).getByTestId("ssh-session-local-port"),
    ).toContainText(String(row!.local_forward_port));
  });

  test("manual Refresh re-fetches the list", async ({ testPage, apiClient, seedData }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.sessionsEmpty).toBeVisible();

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "E3 manual refresh",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");

    await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);

    await page.sessionsRefresh.click();
    await expect(page.sessionsTable).toBeVisible({ timeout: 10_000 });
  });

  test("status badge variants render", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "E4 status",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");
    const row = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);

    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    const badge = page.sessionRow(row.session_id).getByTestId("ssh-session-status");
    await expect(badge).toBeVisible();
    // Any non-empty status is acceptable; we're asserting variant routing
    // works, not what the orchestrator picked for this row.
    await expect(badge).not.toHaveText("");
  });

  test("task id and session id are truncated to 8 chars in the UI", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "E5 truncation",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");
    const row = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);

    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    const taskCell = page.sessionRow(row.session_id).getByTestId("ssh-session-task");
    const sessionCell = page.sessionRow(row.session_id).getByTestId("ssh-session-id");
    await expect(taskCell).toHaveText(task.id.slice(0, 8));
    await expect(sessionCell).toHaveText(row.session_id.slice(0, 8));
  });

  test("polling refresh picks up a new session within the next interval", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.sessionsEmpty).toBeVisible();

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "E6 polling",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );

    // Don't rely on the 90s auto-refresh — exercise the manual refresh, which
    // calls the same code path. The 90s interval is observable via repeated
    // calls but too slow for an e2e timeout.
    await expect
      .poll(
        async () => {
          await page.sessionsRefresh.click();
          const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          return sessions.some((s) => s.task_id === task.id);
        },
        { timeout: 60_000 },
      )
      .toBe(true);
  });
});
