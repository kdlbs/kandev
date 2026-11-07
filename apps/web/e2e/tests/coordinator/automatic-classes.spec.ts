// Raisable resume and start_agent classes and the bounded eligibility settings
// (docs/specs/coordinator/requirements/automatic-classes.md): the settings page
// offers the new classes by their own eligibility, shows the five eligibility
// fields with their bounds, refuses an out-of-bounds value before a save and
// persists an accepted one.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;

test.describe("Coordinator automatic classes and eligibility settings", () => {
  test("resume and start_agent are gated by eligibility and the settings persist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Classes Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    await testPage.goto(
      `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=may-do`,
    );

    // Without a record neither new class can be raised; the others cannot be at all.
    for (const action of ["resume", "start_agent"]) {
      await expect(testPage.locator(`#may-do-${action}-automatic`)).toBeDisabled();
      await expect(testPage.getByTestId(`may-do-automatic-note-${action}`)).toContainText(
        "Not eligible",
      );
    }
    await expect(testPage.getByTestId("may-do-automatic-note-message")).toHaveText(
      "Cannot be raised",
    );

    // The five fields show their bounds; the daily limit is one limit for all classes.
    const group = testPage.getByTestId("eligibility-settings");
    await expect(group).toBeVisible();
    await expect(testPage.getByTestId("eligibility-help-daily_limit")).toContainText(
      "One limit per coordinator across all classes",
    );
    await expect(testPage.getByTestId("eligibility-help-daily_limit")).toContainText(
      "Allowed: 1 to 100",
    );

    // An out-of-bounds value is refused before a save.
    const daily = testPage.getByLabel("Daily limit");
    await daily.fill("0");
    await expect(daily).toHaveAttribute("aria-invalid", "true");

    // An accepted value is saved and read back.
    await daily.fill("4");
    await expect(daily).toHaveAttribute("aria-invalid", "false");
    const saved = waitForHttp(testPage, "PUT", SETTINGS_PATH);
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await saved;
    await testPage.reload();
    await expect(testPage.getByLabel("Daily limit")).toHaveValue("4");
  });
});
