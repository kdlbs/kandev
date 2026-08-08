import { test } from "../../fixtures/docker-test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("Docker repository secrets", () => {
  test("injects a repository-approved secret into the Docker terminal", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const secret = await apiClient.createSecret("E2E Docker Repository Secret", "e2e-docker-value");
    await apiClient.updateRepository(seedData.repositoryId, {
      secret_bindings: [{ key: "E2E_DOCKER_SECRET", secret_id: secret.id }],
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker repository secret",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.dockerExecutorProfileId,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for Docker secret session");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Terminal");
      await session.expectTerminalConnected(30_000);
      await session.typeInTerminal(
        'if [ -n "$E2E_DOCKER_SECRET" ]; then printf docker-binding-present; fi',
      );
      await session.expectTerminalHasText("docker-binding-present");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.deleteSecret(secret.id);
    }
  });
});
