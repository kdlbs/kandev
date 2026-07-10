import { test, expect } from "../../fixtures/ssh-test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForLatestSessionDone } from "../../helpers/session";

/**
 * The Executor Settings popover (top-left of the session view) historically
 * surfaced Docker-container and Sprites-sandbox fields. For SSH it surfaces
 * connection target, remote workdir, agentctl process info, and a
 * paste-ready `ssh ...` command. The data is served by /environment/live
 * under a parallel `ssh` block sourced from ExecutorRunning.Metadata, so
 * the assertion is "user opens popover, sees their host and workdir" — no
 * page action needs to wait for the agent process beyond the initial
 * session reaching a done state.
 */
test.describe("ssh executor — settings popover", () => {
  test("shows host, workdir, agentctl ports, and a paste-ready ssh command", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Popover SSH",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for popover SSH session");

    // Pull the persisted session row so we know the exact host/port/workdir
    // the popover should be reflecting back at us; asserting against the
    // backend values keeps the test resilient to per-run worker port drift.
    const sshSessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sshSessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();
    const remoteTaskDir = row!.remote_task_dir!;
    const remoteAgentctlPort = row!.remote_agentctl_port!;
    const sshHost = seedData.sshTarget.host;
    const sshUser = seedData.sshTarget.user;
    const sshPort = seedData.sshTarget.port;

    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();

    await testPage.getByTestId("executor-settings-button").click();
    const popover = testPage.getByTestId("executor-settings-popover");
    await expect(popover).toBeVisible({ timeout: 5_000 });

    // SSH-specific fields rendered by addSshRows in executor-environment-info.
    // Match by visible text so we catch label-vs-value regressions both ways.
    await expect(popover).toContainText("SSH"); // formatExecutorType("ssh")
    await expect(popover).toContainText(`${sshUser}@${sshHost}:${sshPort}`); // Host row
    await expect(popover).toContainText(remoteTaskDir); // Workdir row
    await expect(popover).toContainText(`remote :${remoteAgentctlPort}`); // Agentctl summary
    await expect(popover).toContainText(`ssh -p ${sshPort} ${sshUser}@${sshHost}`); // Shell row
  });
});
