import { expect, test } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerInspectExists } from "../../helpers/docker";
import { startHTTPGitFixture } from "../../helpers/http-git-server";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { execFileSync } from "node:child_process";

function sourcePayload(remoteURL: string) {
  return {
    sources: [
      { kind: "repository", remote_url: remoteURL, provider: "gitlab", base_branch: "main" },
    ],
  };
}

function containerFile(containerID: string, file: string): string {
  return execFileSync("docker", ["exec", containerID, "cat", file], { encoding: "utf8" });
}

async function cleanupFixture(
  fixture: { close: () => Promise<void> },
  releaseBackendEnv?: () => Promise<void>,
): Promise<void> {
  let releaseError: unknown;
  try {
    await releaseBackendEnv?.();
  } catch (error) {
    releaseError = error;
  } finally {
    try {
      await fixture.close();
    } catch (closeError) {
      if (releaseError) {
        throw new AggregateError(
          [releaseError, closeError],
          "failed to release the backend environment and close the HTTP Git fixture",
        );
      }
      throw closeError;
    }
  }
  if (releaseError) throw releaseError;
}

test.describe("Docker executor — attach workspace sources", () => {
  test("materializes an HTTP repository, rejects folders, and reconstructs the sibling", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.setTimeout(240_000);
    const fixture = await startHTTPGitFixture(backend.tmpDir, "docker-second-source");
    let releaseBackendEnv: (() => Promise<void>) | undefined;
    try {
      releaseBackendEnv = await backend.useEnv(fixture.backendEnv);
      const { executors } = await apiClient.listExecutors();
      const dockerExecutor = executors.find((executor) => executor.type === "local_docker");
      expect(dockerExecutor).toBeTruthy();
      const fixtureProfile = await apiClient.createExecutorProfile(dockerExecutor!.id, {
        name: "E2E Docker HTTP Git fixture",
        config: { image_tag: E2E_IMAGE_TAG },
        prepare_script: "",
        cleanup_script: "",
        env_vars: fixture.gitConfigEnvVars,
      });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker remote workspace sources",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: fixtureProfile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for Docker task");
      const before = await apiClient.getTaskEnvironment(task.id);
      expect(before?.container_id).toBeTruthy();

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Files");
      await testPage.getByTestId("files-workspace-actions").click();
      await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).click();
      const dialog = testPage.getByTestId("add-workspace-sources-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("source-mode-local")).toBeVisible();
      await expect(dialog.getByTestId("source-mode-remote")).toBeVisible();
      await expect(dialog.getByRole("button", { name: "Local folder" })).toHaveCount(0);
      await dialog.getByRole("button", { name: "Cancel" }).click();

      const attached = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/workspace-sources`,
        sourcePayload(fixture.remoteURL),
      );
      expect(attached.status).toBe(200);
      expect(
        containerFile(
          before!.container_id!,
          "/workspace/fixture-docker-second-source-main/remote-source.txt",
        ),
      ).toBe("docker-second-source fixture\n");
      await expect(
        session.files
          .getByTestId("file-tree-node")
          .filter({ hasText: "fixture-docker-second-source-main" }),
      ).toBeVisible({ timeout: 30_000 });

      const forgedFolder = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/workspace-sources`,
        {
          sources: [{ kind: "folder", local_path: backend.tmpDir, display_name: "forged-folder" }],
        },
      );
      expect(forgedFolder.status).toBe(422);
      const persisted = await apiClient.getTask(task.id);
      expect(persisted.repositories).toHaveLength(2);
      expect(persisted.workspace_folders ?? []).toHaveLength(0);
      expect(dockerInspectExists(before!.container_id!)).toBe(true);
      expect(() =>
        containerFile(before!.container_id!, "/workspace/forged-folder/remote-source.txt"),
      ).toThrow();

      const reset = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/environment/reset`,
        {},
      );
      expect(reset.status).toBe(200);
      const relaunched = await apiClient.launchSession({
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: fixtureProfile.id,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      });
      await waitForSessionDone(
        apiClient,
        task.id,
        relaunched.session_id,
        "Waiting for Docker relaunch",
      );
      const after = await apiClient.getTaskEnvironment(task.id);
      expect(after?.container_id).toBeTruthy();
      expect(after?.container_id).not.toBe(before?.container_id);
      expect(
        containerFile(
          after!.container_id!,
          "/workspace/fixture-docker-second-source-main/remote-source.txt",
        ),
      ).toBe("docker-second-source fixture\n");
      await testPage.reload();
      await session.waitForLoad();
      await session.clickTab("Files");
      await expect(
        session.files
          .getByTestId("file-tree-node")
          .filter({ hasText: "fixture-docker-second-source-main" }),
      ).toBeVisible({ timeout: 30_000 });
    } finally {
      await cleanupFixture(fixture, releaseBackendEnv);
    }
  });

  test("rolls back a partially materialized remote batch", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const fixture = await startHTTPGitFixture(backend.tmpDir, "docker-rollback-source");
    let releaseBackendEnv: (() => Promise<void>) | undefined;
    try {
      releaseBackendEnv = await backend.useEnv(fixture.backendEnv);
      const { executors } = await apiClient.listExecutors();
      const dockerExecutor = executors.find((executor) => executor.type === "local_docker");
      expect(dockerExecutor).toBeTruthy();
      const fixtureProfile = await apiClient.createExecutorProfile(dockerExecutor!.id, {
        name: "E2E Docker rollback HTTP Git fixture",
        config: { image_tag: E2E_IMAGE_TAG },
        prepare_script: "",
        cleanup_script: "",
        env_vars: fixture.gitConfigEnvVars,
      });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker source rollback",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: fixtureProfile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for Docker rollback task");
      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.container_id).toBeTruthy();
      const failed = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/workspace-sources`,
        {
          sources: [
            ...sourcePayload(fixture.remoteURL).sources,
            {
              kind: "repository",
              remote_url: `${fixture.remoteURL}-missing`,
              provider: "gitlab",
              base_branch: "main",
            },
          ],
        },
      );
      expect(failed.status).toBe(422);
      expect((await apiClient.getTask(task.id)).repositories).toHaveLength(1);
      expect(() =>
        containerFile(
          environment!.container_id!,
          "/workspace/docker-rollback-source-main/remote-source.txt",
        ),
      ).toThrow();
    } finally {
      await cleanupFixture(fixture, releaseBackendEnv);
    }
  });
});
