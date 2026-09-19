import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerFileContent } from "../../helpers/docker";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";

function dockerPathExists(containerId: string, filePath: string): boolean {
  return (
    spawnSync("docker", ["exec", containerId, "test", "-e", filePath], {
      stdio: "ignore",
    }).status === 0
  );
}

test.describe("Docker executor — portable agent configuration", () => {
  test("copies selected files on fresh provisioning and preserves them on warm resume", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);

    const sourceDir = path.join(backend.tmpDir, ".mock-agent");
    const sourceFile = path.join(sourceDir, "settings.json");
    fs.mkdirSync(sourceDir, { recursive: true });
    fs.writeFileSync(sourceFile, '{"source":"first"}\n');
    fs.writeFileSync(path.join(sourceDir, "unselected.json"), "must not be copied\n");

    const { executors } = await apiClient.listExecutors();
    const dockerExecutor = executors.find((executor) => executor.type === "local_docker");
    expect(dockerExecutor).toBeTruthy();
    const profile = await apiClient.createExecutorProfile(dockerExecutor!.id, {
      name: "E2E portable config Docker profile",
      config: {
        image_tag: E2E_IMAGE_TAG,
        agent_config_bundles: JSON.stringify(["mock.settings"]),
      },
      prepare_script: "",
      cleanup_script: "",
      env_vars: seedData.gitConfigEnvVars,
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker portable agent configuration",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for fresh Docker task");

      const fresh = await apiClient.getTaskEnvironment(task.id);
      expect(fresh?.container_id).toBeTruthy();
      expect(dockerFileContent(fresh!.container_id!, "/root/.mock-agent/settings.json")).toBe(
        '{"source":"first"}\n',
      );
      expect(dockerPathExists(fresh!.container_id!, "/root/.mock-agent/unselected.json")).toBe(
        false,
      );

      fs.writeFileSync(sourceFile, '{"source":"second"}\n');
      const warm = await apiClient.launchSession({
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: profile.id,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      });
      await waitForSessionDone(
        apiClient,
        task.id,
        warm.session_id,
        "Waiting for warm Docker resume",
      );

      const resumed = await apiClient.getTaskEnvironment(task.id);
      expect(resumed?.container_id).toBe(fresh?.container_id);
      expect(dockerFileContent(resumed!.container_id!, "/root/.mock-agent/settings.json")).toBe(
        '{"source":"first"}\n',
      );

      const reset = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/environment/reset`,
        {},
      );
      expect(reset.status).toBe(200);
      const relaunched = await apiClient.launchSession({
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: profile.id,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      });
      await waitForSessionDone(
        apiClient,
        task.id,
        relaunched.session_id,
        "Waiting for reset Docker launch",
      );

      const resetEnvironment = await apiClient.getTaskEnvironment(task.id);
      expect(resetEnvironment?.container_id).toBeTruthy();
      expect(resetEnvironment?.container_id).not.toBe(fresh?.container_id);
      expect(
        dockerFileContent(resetEnvironment!.container_id!, "/root/.mock-agent/settings.json"),
      ).toBe('{"source":"second"}\n');
      expect(relaunched.session_id).toBeTruthy();
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });
});
