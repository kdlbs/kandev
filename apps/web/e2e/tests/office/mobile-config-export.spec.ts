import { expect, test } from "../../fixtures/office-fixture";

test("mobile configuration export supports file preview and back", async ({
  testPage,
  officeSeed: _,
}) => {
  await testPage.goto("/office/workspace/settings/export");
  await expect(testPage.getByText("kandev.yml", { exact: true })).toBeVisible({ timeout: 10_000 });

  await testPage.getByText("kandev.yml", { exact: true }).click();
  await expect(testPage.getByRole("button", { name: "Back to export files" })).toBeVisible();
  await expect(testPage.locator("pre")).toContainText("name:");

  await testPage.getByRole("button", { name: "Back to export files" }).click();
  await expect(testPage.getByText("kandev.yml", { exact: true })).toBeVisible();
});
