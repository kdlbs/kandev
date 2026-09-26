import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile Office config sync", () => {
  test("stacks provider fields and saves through the shared settings bar", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.deleteConfigSyncConfig(officeSeed.workspaceId);
    try {
      await testPage.goto("/office/workspace/settings");

      const owner = testPage.getByLabel("Repository owner");
      const name = testPage.getByLabel("Repository name");
      await expect(owner).toBeVisible();
      await expect(name).toBeVisible();

      const ownerBox = await owner.boundingBox();
      const nameBox = await name.boundingBox();
      expect(ownerBox).not.toBeNull();
      expect(nameBox).not.toBeNull();
      expect(nameBox!.y).toBeGreaterThan(ownerBox!.y);

      await owner.fill("kdlbs");
      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });
      await expect(saveButton).toBeDisabled();
      await name.fill("office-config");
      await expect(saveButton).toBeEnabled();
      await saveButton.tap();

      await expect(testPage.getByTestId("office-config-sync-status")).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile Office config sync settings");
    } finally {
      await officeApi.deleteConfigSyncConfig(officeSeed.workspaceId);
    }
  });
});
