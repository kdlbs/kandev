import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { KanbanPage } from "../../pages/kanban-page";
import {
  createMismatchedProfile,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("PR 3473 QA mobile visual evidence", () => {
  test("captures mobile warning surfaces", async ({ testPage, apiClient, prCapture }) => {
    const profile = await createMismatchedProfile(apiClient, "PR 3473 mobile visual profile");
    try {
      const { agents } = await apiClient.listAgents();
      const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
      if (!agent) throw new Error("The E2E fixture must provide a mock agent");

      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const modelSelector = testPage.getByRole("button", {
        name: "Profile start model settings",
      });
      await expect(modelSelector).toBeVisible({ timeout: 15_000 });
      await expect(modelSelector).toContainText(UNADVERTISED_MODEL);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile profile start model unavailable");
      await prCapture.screenshot("mobile-settings-start-model-unavailable", {
        caption:
          "Mobile profile settings preserve the exact saved model without horizontal overflow.",
        fullPage: true,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await testPage.getByRole("button", { name: "Add task" }).tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("agent-profile-selector").tap();
      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      await warning.tap();
      const warningText = `The host probe did not advertise ${UNADVERTISED_MODEL}. The selected executor will decide the model at launch.`;
      const warningDrawer = testPage
        .locator('[data-slot="drawer-content"][data-state="open"]')
        .filter({ hasText: warningText });
      await expect(warningDrawer).toBeVisible();
      await expect(warningDrawer).toHaveCSS("transform", "none");
      expect(await warningDrawer.evaluate((element) => getComputedStyle(element).zIndex)).toBe(
        "80",
      );
      await assertNoDocumentHorizontalOverflow(testPage, "mobile task create host probe warning");
      await prCapture.screenshot("mobile-task-create-host-probe-warning", {
        caption:
          "Mobile task creation keeps the exact-profile option touch-selectable and opens the advisory warning in a drawer.",
      });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined);
    }
  });
});
