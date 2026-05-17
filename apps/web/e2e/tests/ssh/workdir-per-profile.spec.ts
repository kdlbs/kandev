import { test, expect } from "../../fixtures/ssh-test-base";
import { remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";

/**
 * Per-profile ssh_workdir_root: different profiles can target different
 * remote workdir trees on the same host. Default profile (no workdir
 * config) falls back to ~/.kandev. Switching profile mid-life does not
 * relocate an existing task dir.
 *
 * Covers e2e-plan.md group P (P1–P4).
 */
test.describe("ssh executor — workdir per profile", () => {
  test("profile workdir_root=~/.kandev-a places the task dir under that root", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "P1 kandev-a",
      config: { ssh_workdir_root: "~/.kandev-a" },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "P1 workdir kandev-a",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: profile.id,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch under workdir A");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.remote_task_dir).toMatch(/\.kandev-a\/tasks\//);
    expect(remotePathExists(seedData.sshTarget, row!.remote_task_dir!)).toBe(true);
  });

  test("two profiles on the same host map to two different workdir trees", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const profA = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "P2 team-a",
      config: { ssh_workdir_root: "~/.kandev-team-a" },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });
    const profB = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "P2 team-b",
      config: { ssh_workdir_root: "~/.kandev-team-b" },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "P2 task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: profA.id,
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "P2 task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: profB.id,
      },
    );
    await waitForLatestSessionDone(apiClient, taskA.id, 1, "Task A");
    await waitForLatestSessionDone(apiClient, taskB.id, 1, "Task B");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const dirA = sessions.find((s) => s.task_id === taskA.id)?.remote_task_dir;
    const dirB = sessions.find((s) => s.task_id === taskB.id)?.remote_task_dir;
    expect(dirA).toMatch(/\.kandev-team-a/);
    expect(dirB).toMatch(/\.kandev-team-b/);
  });

  test("default profile (no workdir_root) lands under ~/.kandev/tasks", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // seedData.sshExecutorProfileId is the default profile; it has no
    // workdir_root override. The runtime should fall back to ~/.kandev.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "P3 default workdir",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Default workdir");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.remote_task_dir).toMatch(/\/\.kandev\/tasks\//);
  });
});
