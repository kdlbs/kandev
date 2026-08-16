import { expect, test } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  createMismatchedProfile,
  readModelSelectionWarnings,
  UNADVERTISED_MODEL,
} from "./model-mismatch-warning-helpers";

test.describe("model mismatch warning", () => {
  test("continues with the executor default and persists one warning after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const profile = await createMismatchedProfile(apiClient, "Executor default warning profile");
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Executor model mismatch warning",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      expect(task.session_id).toBeTruthy();
      await waitForSessionDone(
        apiClient,
        task.id,
        task.session_id!,
        "Waiting for model mismatch task continuation",
      );

      await expect
        .poll(() => readModelSelectionWarnings(apiClient, task.session_id!), {
          timeout: 15_000,
          message: "Waiting for the persisted model-selection warning",
        })
        .toHaveLength(1);
      const warnings = await readModelSelectionWarnings(apiClient, task.session_id!);
      expect(warnings[0]?.content).toContain("saved model selection");
      expect(warnings[0]?.metadata).toMatchObject({
        kind: "model_selection_warning",
        reason: "requested_not_advertised",
        requested_model: UNADVERTISED_MODEL,
        agent_id: "mock-agent",
      });
      expect(warnings[0]?.metadata?.effective_model).toBe("mock-fast");
      expect((await apiClient.getAgentProfile(profile.id)).model).toBe(UNADVERTISED_MODEL);

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("The executor could not use the saved model selection."),
      ).toBeVisible({ timeout: 15_000 });
      await expect(session.activeChat()).toContainText(UNADVERTISED_MODEL);

      await testPage.reload();
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("The executor could not use the saved model selection."),
      ).toBeVisible({ timeout: 15_000 });
      await expect(session.activeChat()).toContainText("Mock Fast");
      await expect
        .poll(() => readModelSelectionWarnings(apiClient, task.session_id!))
        .toHaveLength(1);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
