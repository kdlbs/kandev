import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  createMismatchedProfile,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("executor-authoritative model selection", () => {
  test("keeps a host-mismatched profile selectable", async ({ testPage, apiClient }) => {
    const profile = await createMismatchedProfile(apiClient, "Host mismatch selectable profile");
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const selector = dialog.getByTestId("agent-profile-selector");
      await expect(selector).toBeVisible();
      await selector.click();

      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      await expect(option).toBeEnabled();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      const warningText = `The host probe did not advertise ${UNADVERTISED_MODEL}. The selected executor will decide the model at launch.`;
      await expect(dialog).not.toContainText(warningText);
      await warning.hover();
      await expect(
        testPage
          .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
          .filter({ hasText: warningText }),
      ).toBeVisible();
      await option.click();
      await expect(selector.locator("button")).toHaveCount(0);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
