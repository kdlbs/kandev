import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  createModelVariationProfile,
  createMismatchedProfile,
  MODEL_VARIATION_BASE,
  UNIQUE_MODEL_VARIATION,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("executor-authoritative model selection", () => {
  test("saves exact mode and restores dormant fallback settings", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    if (!agent) throw new Error("The E2E fixture must provide an agent");
    const profile = await apiClient.createAgentProfile(agent.id, "Exact model settings profile", {
      model: "mock-fast",
      fallback_model: "mock-smart",
      auto_fallback: false,
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const trigger = testPage.getByTestId("profile-fallback-settings-trigger");
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await trigger.click();

      const exactSwitch = testPage.getByRole("switch", { name: "Require exact model" });
      await expect(exactSwitch).toHaveAttribute("data-state", "unchecked");
      await exactSwitch.click();
      await expect(exactSwitch).toHaveAttribute("data-state", "checked");
      await expect(
        testPage.getByRole("switch", { name: "Fallback automatically to next model" }),
      ).toBeDisabled();
      await expect(testPage.getByRole("switch", { name: "Agent fallback" })).toBeDisabled();
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Exact model required",
      );

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          requireExactModel: true,
          fallbackModel: "mock-smart",
        });

      await testPage.reload();
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Exact model required",
      );
      await trigger.click();
      await expect(testPage.getByRole("switch", { name: "Require exact model" })).toHaveAttribute(
        "data-state",
        "checked",
      );
      await testPage.getByRole("switch", { name: "Require exact model" }).click();
      const saveAfterDisable = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await saveAfterDisable.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          requireExactModel: false,
          fallbackModel: "mock-smart",
        });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

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
      expect(
        await testPage
          .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
          .filter({ hasText: warningText })
          .evaluate((element) => getComputedStyle(element).zIndex),
      ).toBe("80");
      await option.click();
      await expect(selector.locator("button")).toHaveCount(0);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

  test("does not infer one host-advertised variation while keeping the profile selectable", async ({
    testPage,
    apiClient,
  }) => {
    const profile = await createModelVariationProfile(
      apiClient,
      "No inferred host variation selectable profile",
      "unique",
    );
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.click();

      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      await expect(option).toBeEnabled();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      const warningText = `The host probe did not advertise ${MODEL_VARIATION_BASE}. The selected executor will decide the model at launch.`;
      await expect(warning).not.toHaveAccessibleName(UNIQUE_MODEL_VARIATION);
      await warning.hover();
      await expect(
        testPage
          .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
          .filter({ hasText: warningText }),
      ).toBeVisible();
      await option.click();
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
