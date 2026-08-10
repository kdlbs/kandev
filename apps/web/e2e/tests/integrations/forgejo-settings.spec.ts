import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

async function gotoForgejoSettings(page: import("@playwright/test").Page) {
  await page.goto("/settings/integrations/forgejo");
  await page.getByTestId("forgejo-origin-input").waitFor();
}

test.describe("Forgejo settings", () => {
  test("shows the workspace-scoped connection form and shared unsaved state", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo`);
    await testPage.getByTestId("forgejo-origin-input").waitFor();
    await expect(testPage.getByTestId("forgejo-origin-input")).toHaveValue("");
    await expect(testPage.getByTestId("forgejo-token-input")).toHaveValue("");
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo$`),
    );

    await testPage.getByTestId("forgejo-origin-input").fill("https://forgejo.example");
    await testPage.getByTestId("forgejo-token-input").fill("forgejo-token");
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  });

  test("keeps Forgejo credential controls usable on a mobile viewport", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await gotoForgejoSettings(testPage);

    await expect(testPage.getByTestId("forgejo-origin-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-token-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-webhook-secret-input")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Forgejo settings");
  });
});
