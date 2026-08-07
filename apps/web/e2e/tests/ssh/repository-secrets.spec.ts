import { test } from "../../fixtures/ssh-test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("SSH repository secrets", () => {
  test("forwards approved repository keys but not unrelated profile keys", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const secret = await apiClient.createSecret("E2E SSH Repository Secret", "e2e-ssh-value");
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "E2E SSH Repository Secret Boundary",
      config: {},
      prepare_script: "",
      cleanup_script: "",
      env_vars: [{ key: "E2E_UNRELATED_PROFILE", value: "not-forwarded" }],
    });
    await apiClient.updateRepository(seedData.repositoryId, {
      secret_bindings: [{ key: "E2E_SSH_SECRET", secret_id: secret.id }],
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "SSH repository secret",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for SSH secret session");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Terminal");
      await session.expectTerminalConnected(30_000);
      await session.typeInTerminal(
        'if [ -n "$E2E_SSH_SECRET" ]; then printf ssh-approved-present; fi',
      );
      await session.expectTerminalHasText("ssh-approved-present");
      await session.typeInTerminal(
        'if [ -n "$E2E_UNRELATED_PROFILE" ]; then printf unrelated-present; else printf unrelated-absent; fi',
      );
      await session.expectTerminalHasText("unrelated-absent");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.deleteSecret(secret.id);
      await apiClient.deleteExecutorProfile(profile.id);
    }
  });
});
