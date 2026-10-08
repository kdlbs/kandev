// Routing: /t/{taskId}. Verifies the retired built-in Prompt history panel is
// gone from the desktop workbench "+" menu. The case that proves a saved
// layout still carrying the retired entry restores cleanly lives in
// tests/settings/layout-profiles.spec.ts.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Rows a default task's "+" menu renders, by accessible name. Changes and Files
 * rows are deliberately absent from this list: the built-in default preset
 * opens both panels, and those rows only render while their panel is closed.
 */
const OFFERED_ROWS = ["Plan", "Todos", "VS Code", "Browser"];

test.describe("Prompt history removed from the task workbench", () => {
  test("offers no Prompt history row in the + menu", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Prompt history removed task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await session.addPanelButton().click();

    // Wait for a real row first: an unopened menu would make the absence
    // assertions below pass for the wrong reason.
    for (const label of OFFERED_ROWS) {
      await expect(testPage.getByRole("menuitem", { name: label, exact: true })).toBeVisible();
    }

    // The row's own test id no longer exists in the tree, so the label check is
    // the assertion with power; it is exact so a plugin panel whose title
    // merely starts with "Prompt history" cannot satisfy it.
    await expect(testPage.getByTestId("add-panel-prompt-history-item")).toHaveCount(0);
    await expect(
      testPage.getByRole("menuitem", { name: "Prompt history", exact: true }),
    ).toHaveCount(0);
  });
});
