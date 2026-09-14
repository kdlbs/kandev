import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  createExecutorOnlyModelProfile,
  createModelVariationProfile,
  createMismatchedProfile,
} from "../session/model-mismatch-warning-helpers";
import { launchExecutorOnlyModelProfile } from "./profile-model-selection-helpers";

test.describe("executor-authoritative model selection", () => {
  test("keeps a host-mismatched profile selectable", async ({ testPage, apiClient, prCapture }) => {
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
      await expect(option.getByTestId("agent-profile-model-probe-warning")).toHaveCount(0);
      await expect(option.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
      await prCapture.screenshot("desktop-profile-options", {
        caption: "Saved profiles remain selectable without host-model warnings.",
      });
      const search = testPage.locator("[cmdk-input]");
      await search.fill(profile.name);
      await search.press("Enter");
      await expect(testPage.getByRole("listbox")).not.toBeVisible();
      await expect(selector).toContainText(profile.name);
      await prCapture.screenshot("desktop-selected-profile", {
        caption: "The selected profile keeps its name without a model advisory.",
      });
      await expect(selector.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
      await expect(selector.locator("button")).toHaveCount(0);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

  test("selects a unique-variation profile without a host model advisory", async ({
    testPage,
    apiClient,
  }) => {
    const profile = await createModelVariationProfile(
      apiClient,
      "Unique host variation selectable profile",
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
      await expect(option.getByTestId("agent-profile-model-probe-warning")).toHaveCount(0);
      await expect(option.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
      await option.click();
      await expect(testPage.getByRole("listbox")).not.toBeVisible();
      await expect(selector).toContainText(profile.name);
      await expect(selector.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });
  // @covers AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.5
  // @covers AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.6
  test("launches a host-mismatched profile on its requested executor model without a warning", async ({
    testPage,
    apiClient,
  }) => {
    const profile = await createExecutorOnlyModelProfile(apiClient);
    try {
      await launchExecutorOnlyModelProfile(testPage, apiClient, profile, false);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
