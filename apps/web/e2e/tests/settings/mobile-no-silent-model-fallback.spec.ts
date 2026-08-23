import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createMismatchedProfile,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("executor-authoritative model selection on mobile", () => {
  test("keeps the host-mismatched profile reachable by touch", async ({ testPage, apiClient }) => {
    const profile = await createMismatchedProfile(apiClient, "Mobile host mismatch profile");
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await testPage.getByRole("button", { name: "Add task" }).tap();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const selector = dialog.getByTestId("agent-profile-selector");
      await expect(selector).toBeVisible();
      await selector.tap();

      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      await expect(option).toBeEnabled();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      const warningText = `The host probe did not advertise ${UNADVERTISED_MODEL}. The selected executor will decide the model at launch.`;
      await expect(dialog).not.toContainText(warningText);
      await warning.tap();
      await expect(
        testPage
          .locator('[data-slot="drawer-content"][data-state="open"]')
          .filter({ hasText: warningText }),
      ).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile model mismatch selector");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
