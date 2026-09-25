import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  createFixtureExecutorProfile,
  installFixtureExecutorProvider,
  readFixtureProfile,
} from "../../helpers/plugin-executor-profile";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";

useRegularMode();

async function expectTouchTarget(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(box!.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
}

test("phone users can configure and select a remote provider profile", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  try {
    const executor = await installFixtureExecutorProvider(testPage, apiClient);
    await testPage.goto("/settings/executors");
    const profile = await createFixtureExecutorProfile(testPage, executor, true);
    const editor = testPage.getByTestId("plugin-executor-profile-page");
    const nameField = testPage.locator("#executor-profile-name");
    const retention = testPage.getByTestId("executor-profile-retention");
    await expect(editor).toBeVisible();
    await expectTouchTarget(nameField, "Profile name field");
    await expectTouchTarget(testPage.getByLabel("Region"), "Region selector");
    await expectTouchTarget(testPage.getByLabel("Credential"), "Credential field");
    await expect(editor).toContainText("Maximum lifetime: 8 hours.");
    await assertNoDocumentHorizontalOverflow(testPage, "provider profile page");

    await testPage.setViewportSize({ width: 767, height: 900 });
    const nameAt767 = await nameField.boundingBox();
    const retentionAt767 = await retention.boundingBox();
    expect(nameAt767).not.toBeNull();
    expect(retentionAt767).not.toBeNull();
    expect(retentionAt767!.y).toBeGreaterThan(nameAt767!.y + nameAt767!.height - 1);
    await testPage.setViewportSize({ width: 768, height: 900 });
    const nameAt768 = await nameField.boundingBox();
    const retentionAt768 = await retention.boundingBox();
    expect(nameAt768).not.toBeNull();
    expect(retentionAt768).not.toBeNull();
    expect(retentionAt768!.x).toBeGreaterThan(nameAt768!.x + 100);
    expect(retentionAt768!.y).toBeLessThan(nameAt768!.y + nameAt768!.height);
    await assertNoDocumentHorizontalOverflow(testPage, "provider profile at the 768px breakpoint");

    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage
      .getByLabel(/Environment label with additional context/)
      .fill("mobile-workspace-name-that-wraps-cleanly");
    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expectTouchTarget(saveButton, "Floating profile save button");
    const labelSaved = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/api/v1/executors/${executor.id}/profiles/${profile.id}$`),
      { predicate: (response) => response.ok() },
    );
    await saveButton.tap();
    await labelSaved;
    await testPage.reload();
    await expect(testPage.getByLabel(/Environment label with additional context/)).toHaveValue(
      "mobile-workspace-name-that-wraps-cleanly",
    );

    await testPage.getByTestId("executor-profile-secret-clear-credential").tap();
    const secretCleared = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/api/v1/executors/${executor.id}/profiles/${profile.id}$`),
      { predicate: (response) => response.ok() },
    );
    await saveButton.tap();
    await secretCleared;
    const publicProfile = await readFixtureProfile(apiClient, executor.id, profile.id);
    expect(publicProfile.secret_fields?.credential).not.toBe(true);

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await kanban.mobileFab.tap();
    const selector = testPage.getByTestId("executor-profile-selector");
    await expect(selector).toBeVisible();
    await expectTouchTarget(selector, "Executor profile picker trigger");
    await selector.tap();
    const drawer = testPage.getByTestId("executor-profile-selector-dropdown");
    await expect(drawer).toBeVisible();
    const commandList = drawer.locator("[data-vaul-no-drag]");
    await expect(commandList).toBeVisible();
    await expect
      .poll(() => commandList.evaluate((element) => getComputedStyle(element).overflowY))
      .toBe("auto");
    const option = drawer.locator(`[cmdk-item][data-value="${profile.id}"]`);
    await expect(option).toContainText("Maximum lifetime: 8 hours.");
    await expectTouchTarget(option, "Provider profile option");
    await option.tap();
    await expect(selector).toContainText(profile.name);
    await expect(selector).toBeFocused();
    await assertNoDocumentHorizontalOverflow(testPage, "provider profile picker drawer");
  } finally {
    await uninstallFixturePlugin(apiClient);
  }
});
