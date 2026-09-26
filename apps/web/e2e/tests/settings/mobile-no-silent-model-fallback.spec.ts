import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createModelVariationProfile,
  createMismatchedProfile,
  MODEL_VARIATION_BASE,
  UNIQUE_MODEL_VARIATION,
  UNADVERTISED_MODEL,
} from "../session/model-mismatch-warning-helpers";

test.describe("executor-authoritative model selection on mobile", () => {
  test("saves exact mode and restores dormant fallback settings by touch", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    if (!agent) throw new Error("The E2E fixture must provide an agent");
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile exact model settings", {
      model: "mock-fast",
      fallback_model: "mock-smart",
      auto_fallback: false,
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const trigger = testPage.getByTestId("profile-fallback-settings-trigger");
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await trigger.tap();
      const exactSwitch = testPage.getByRole("switch", { name: "Require exact model" });
      await expect(exactSwitch).toHaveAttribute("data-state", "unchecked");
      await exactSwitch.tap();
      await expect(exactSwitch).toHaveAttribute("data-state", "checked");
      await expect(testPage.getByRole("switch", { name: "Agent fallback" })).toBeDisabled();
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Exact model required",
      );
      await expect(
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).resolves.toBe(true);

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await saveButton.tap();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          requireExactModel: true,
          fallbackModel: "mock-smart",
        });

      await testPage.reload();
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await trigger.tap();
      await testPage.getByRole("switch", { name: "Require exact model" }).tap();
      await testPage
        .getByRole("button", { name: /^Save( changes)?$/i })
        .first()
        .tap();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          requireExactModel: false,
          fallbackModel: "mock-smart",
        });
      await expect(
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).resolves.toBe(true);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

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
      expect(
        await testPage
          .locator('[data-slot="drawer-content"][data-state="open"]')
          .filter({ hasText: warningText })
          .evaluate((element) => getComputedStyle(element).zIndex),
      ).toBe("80");
      await assertNoDocumentHorizontalOverflow(testPage, "mobile model mismatch selector");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

  test("does not infer one host-advertised variation by touch without overflow", async ({
    testPage,
    apiClient,
  }) => {
    const profile = await createModelVariationProfile(
      apiClient,
      "Mobile no inferred host variation profile",
      "unique",
    );
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await testPage.getByRole("button", { name: "Add task" }).tap();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.tap();

      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: profile.name, exact: false });
      await expect(option).toBeVisible();
      await expect(option).toBeEnabled();
      const warning = option.getByTestId("agent-profile-model-probe-warning");
      await expect(warning).toBeVisible();
      const warningText = `The host probe did not advertise ${MODEL_VARIATION_BASE}. The selected executor will decide the model at launch.`;
      await expect(warning).not.toHaveAccessibleName(UNIQUE_MODEL_VARIATION);
      await warning.tap();
      await expect(
        testPage
          .locator('[data-slot="drawer-content"][data-state="open"]')
          .filter({ hasText: warningText }),
      ).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile no inferred variation advisory");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
});
