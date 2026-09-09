import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createExecutorOnlyModelProfile,
  createModelVariationProfile,
  createMismatchedProfile,
} from "../session/model-mismatch-warning-helpers";
import { launchExecutorOnlyModelProfile } from "./profile-model-selection-helpers";

test.describe("executor-authoritative model selection on mobile", () => {
  test("keeps the host-mismatched profile reachable by touch", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
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
      await expect(option.getByTestId("agent-profile-model-probe-warning")).toHaveCount(0);
      await expect(option.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
      await prCapture.screenshot("mobile-profile-options", {
        caption: "Saved profiles remain selectable without host-model warnings.",
      });
      await option.tap();
      await expect(testPage.getByRole("listbox")).not.toBeVisible();
      await expect(selector).toContainText(profile.name);
      await prCapture.screenshot("mobile-selected-profile", {
        caption: "The selected profile keeps its name without a model advisory.",
      });
      await expect(selector.locator("button, .tabler-icon-alert-triangle")).toHaveCount(0);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile profile selection");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
    }
  });

  test("selects a unique-variation profile by touch without model help", async ({
    testPage,
    apiClient,
  }) => {
    const profile = await createModelVariationProfile(
      apiClient,
      "Mobile unique host variation profile",
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
      await expect(option.getByTestId("agent-profile-model-probe-warning")).toHaveCount(0);
      await expect(option.locator(".tabler-icon-alert-triangle")).toHaveCount(0);
      await option.tap();
      await expect(testPage.getByRole("listbox")).not.toBeVisible();
      await expect(selector).toContainText(profile.name);
      await expect(selector.locator("button, .tabler-icon-alert-triangle")).toHaveCount(0);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile profile selection");
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
      await launchExecutorOnlyModelProfile(testPage, apiClient, profile, true);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
