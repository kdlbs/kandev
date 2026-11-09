import fs from "node:fs";

import { expect } from "@playwright/test";

import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export type DisabledDockerRefusal = {
  taskId: string;
  profileId: string;
  cleanup: () => Promise<void>;
};

/**
 * Creates a Local Docker profile on a backend where the Docker runtime is
 * disabled and launches a task that selects it. The helper waits for the
 * backend's causal refusal, then proves no session, environment, or observed
 * runtime was created and that the saved selection remains intact.
 */
export async function createDisabledDockerRefusal(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
): Promise<DisabledDockerRefusal | null> {
  const { executors } = await apiClient.listExecutors();
  const docker = executors.find((executor) => executor.type === "local_docker");
  if (!docker) {
    return null;
  }

  const profile = await apiClient.createExecutorProfile(docker.id, {
    name: `Disabled Docker ${Date.now()}`,
    config: { image_tag: "kandev/e2e-disabled-docker:latest" },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });

  const logOffset = fs.existsSync(backend.logPath) ? fs.statSync(backend.logPath).size : 0;

  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Disabled Docker refusal ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: profile.id,
      },
    );

    // The refusal is causal: wait for a new backend record for this exact task
    // before asserting that no worker identity exists. Reading from the
    // pre-launch offset prevents an earlier test's refusal from satisfying it.
    await expect
      .poll(
        () => {
          const chunk = readLogSince(backend.logPath, logOffset);
          return (
            chunk.includes(task.id) &&
            /executor runtime .*unavailable|executor_runtime_unavailable/.test(chunk)
          );
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions, "a refused Docker launch must not create a session").toHaveLength(0);

    const environment = await apiClient.getTaskEnvironment(task.id).catch(() => null);
    expect(environment, "a refused Docker launch must not create an environment").toBeNull();

    const current = await apiClient.getTask(task.id);
    expect(current.metadata?.executor_profile_id).toBe(profile.id);

    return {
      taskId: task.id,
      profileId: profile.id,
      cleanup: async () => {
        await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
      },
    };
  } catch (error) {
    await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    throw error;
  }
}

function readLogSince(filePath: string, offset: number): string {
  if (!fs.existsSync(filePath)) return "";
  const size = fs.statSync(filePath).size;
  const start = size >= offset ? offset : 0;
  const length = size - start;
  if (length <= 0) return "";
  const fd = fs.openSync(filePath, "r");
  try {
    const buffer = Buffer.alloc(length);
    fs.readSync(fd, buffer, 0, length, start);
    return buffer.toString("utf8");
  } finally {
    fs.closeSync(fd);
  }
}
