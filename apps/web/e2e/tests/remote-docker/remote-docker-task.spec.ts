import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/remote-docker-test-base";
import { waitForSessionDone, waitForLatestSessionDone } from "../../helpers/session";

/**
 * Full task lifecycle on a Docker daemon reached over SSH.
 *
 * This is the scenario unit tests cannot reach. It exercises the SSH
 * transport, the remote-resolved mount sources, and the forwarded agentctl
 * port together — and, most importantly, that an ordinary stop preserves the
 * remote container so a resume reattaches to it instead of launching a second
 * one and abandoning the workspace.
 */
function containerExists(containerId: string): boolean {
  const out = execFileSync("docker", ["ps", "-aq", "--no-trunc", "--filter", `id=${containerId}`], {
    encoding: "utf8",
  })
    .toString()
    .trim();
  return out !== "";
}

test.describe("remote docker executor — task lifecycle", () => {
  test("launches a container on the remote daemon and reattaches on resume", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "remote docker launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the remote Docker session");

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.executor_type).toBe("remote_docker");
    expect(env!.container_id, "no container recorded for the task").toBeTruthy();

    // The container lives on the daemon the SSH host exposes, which in this
    // fixture is the machine's own daemon.
    expect(containerExists(env!.container_id!)).toBe(true);

    // A second session resumes into the preserved container. A fresh launch
    // here would abandon the workspace the user expects to return to.
    const warm = await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.remoteDockerExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:simple-message",
    });
    await waitForSessionDone(apiClient, task.id, warm.session_id, "Wait for the resumed session");

    const resumed = await apiClient.getTaskEnvironment(task.id);
    expect(resumed?.container_id, "resume launched a new container instead of reattaching").toBe(
      env!.container_id,
    );
  });

  test("removes the remote container when the task is deleted", async ({ apiClient, seedData }) => {
    test.setTimeout(300_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "remote docker teardown",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the remote Docker session");
    const env = await apiClient.getTaskEnvironment(task.id);
    const containerId = env?.container_id;
    expect(containerId).toBeTruthy();

    await apiClient.deleteTask(task.id);

    // A terminal stop has to reach the daemon over SSH and remove the
    // container. Preserving here would strand it on the remote host.
    await expect
      .poll(() => containerExists(containerId!), {
        message: "container survived task deletion on the remote daemon",
        timeout: 90_000,
      })
      .toBe(false);
  });

  test("writes nothing to the remote host filesystem", async ({ apiClient, seedData }) => {
    test.setTimeout(300_000);

    // The SSH host's $HOME is this directory, mounted in at the same absolute
    // path, so the test runner can read what a launch left on the remote
    // account. Before container-native delivery a launch created
    // .kandev/bin/agentctl and .kandev/agent-sessions/<instance>/ here, and
    // the session directory held the agent's credential files.
    const remoteHome = path.join(seedData.remoteHost.workDir, "remote-home");
    const kandevTree = path.join(remoteHome, ".kandev");

    // Without this the assertions below pass just as happily against a path
    // that never existed, which is the way this test could quietly stop
    // checking anything.
    expect(fs.existsSync(remoteHome), `${remoteHome} is not the SSH host's home`).toBe(true);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "remote docker host footprint",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the remote Docker session");
    expect(fs.existsSync(kandevTree), `launch created ${kandevTree} on the remote host`).toBe(
      false,
    );

    // A resume re-delivers the agentctl helper. It must do that through the
    // daemon too, not by caching it on the host.
    const warm = await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.remoteDockerExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:simple-message",
    });
    await waitForSessionDone(apiClient, task.id, warm.session_id, "Wait for the resumed session");
    expect(fs.existsSync(kandevTree), `resume created ${kandevTree} on the remote host`).toBe(
      false,
    );

    await apiClient.deleteTask(task.id);
    expect(fs.existsSync(kandevTree), `teardown left ${kandevTree} on the remote host`).toBe(false);
  });
});
