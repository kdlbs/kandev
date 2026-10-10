import { dockerTest as test, expect } from "../../fixtures/docker-test-base";
import { dockerInspectExists, dockerRemove } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";
import { enableTurnChangedFiles, waitForTurnChangeCount } from "./turn-changed-files-helpers";

test.describe("Docker executor turn change retention", () => {
  test("keeps exported turn content after container removal and backend restart", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker turn change retention",
        seedData.agentProfileId,
        {
          description: "/e2e:untracked-file-setup",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.dockerExecutorProfileId,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for Docker turn capture");
      expect(task.session_id).toBeTruthy();
      const sessionId = task.session_id!;
      await waitForTurnChangeCount(apiClient, sessionId, 1);

      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.container_id).toBeTruthy();
      const containerId = environment!.container_id!;
      expect(dockerInspectExists(containerId)).toBe(true);

      const historyResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/task-sessions/${sessionId}/turn-changes?offset=0&limit=20`,
      );
      expect(historyResponse.ok).toBe(true);
      const history = (await historyResponse.json()) as {
        change_sets: Array<{ id: string; repositories: Array<{ id: string }> }>;
      };
      const changeSet = history.change_sets[0];
      const repository = changeSet?.repositories[0];
      expect(changeSet?.id).toBeTruthy();
      expect(repository?.id).toBeTruthy();

      const filesResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSet!.id}/repositories/${repository!.id}/files?offset=0&limit=20`,
      );
      expect(filesResponse.ok).toBe(true);
      const filePage = (await filesResponse.json()) as {
        files: Array<{ id: string; path: string }>;
      };
      const file = filePage.files.find((entry) => entry.path === "untracked_test.txt");
      expect(file?.id).toBeTruthy();

      dockerRemove(containerId);
      expect(dockerInspectExists(containerId)).toBe(false);
      await backend.restart();
      await backend.ensureReady();

      const contentResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSet!.id}/files/${file!.id}/content?variant=canonical_patch`,
      );
      expect(contentResponse.ok).toBe(true);
      const content = (await contentResponse.json()) as { content: string };
      expect(Buffer.from(content.content, "base64").toString("utf8")).toContain("INITIAL_CONTENT");
    } finally {
      await restorePreference();
    }
  });
});
