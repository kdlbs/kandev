import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  createMismatchedProfile,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("PR 3473 QA visual evidence", () => {
  test("captures desktop warning surfaces", async ({ testPage, apiClient, prCapture }) => {
    const profile = await createMismatchedProfile(apiClient, "PR 3473 desktop visual profile");
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
      await prCapture.screenshot("desktop-settings-start-model-unavailable", {
        caption:
          "Desktop profile settings preserve the exact saved model instead of silently substituting a host-advertised model.",
        fullPage: true,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("agent-profile-selector").click();
      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      await warning.hover();
      const warningText = `The host probe did not advertise ${UNADVERTISED_MODEL}. The selected executor will decide the model at launch.`;
      const warningTooltip = testPage
        .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
        .filter({ hasText: warningText });
      await expect(warningTooltip).toBeVisible();
      await expect(warningTooltip).toHaveCSS("opacity", "1");
      expect(await warningTooltip.evaluate((element) => getComputedStyle(element).zIndex)).toBe(
        "80",
      );
      await prCapture.screenshot("desktop-task-create-host-probe-warning", {
        caption:
          "Desktop task creation keeps the exact-profile option selectable and scopes the host-probe warning to an advisory tooltip.",
      });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined);
    }
  });
});
