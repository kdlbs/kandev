import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  createMismatchedProfile,
  readModelSelectionWarnings,
  UNADVERTISED_MODEL,
} from "./model-mismatch-warning-helpers";

test.describe("model mismatch warning on mobile", () => {
  test("shows the persisted warning without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const profile = await createMismatchedProfile(apiClient, "Mobile executor warning profile");
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mobile executor model mismatch warning",
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
        "Waiting for mobile model mismatch task continuation",
      );
      await expect
        .poll(() => readModelSelectionWarnings(apiClient, task.session_id!), {
          timeout: 15_000,
        })
        .toHaveLength(1);

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("The executor could not use the saved model selection."),
      ).toBeVisible({ timeout: 15_000 });
      await expect(session.activeChat()).toContainText(UNADVERTISED_MODEL);
      await expect(session.activeChat()).toContainText("Mock Fast");
      await assertNoDocumentHorizontalOverflow(testPage, "mobile model mismatch warning");

      await testPage.reload();
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("The executor could not use the saved model selection."),
      ).toBeVisible({ timeout: 15_000 });
      await assertNoDocumentHorizontalOverflow(testPage, "reloaded mobile model mismatch warning");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
