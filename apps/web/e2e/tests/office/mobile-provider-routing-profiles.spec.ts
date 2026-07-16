import { test, expect } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

test.describe("Office execution profile routing on mobile", () => {
  test("keeps the first-provider profile selector usable with fallback disabled", async ({
    testPage,
    backend,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
    const configured = await balancedExecutionProfileRouting(
      apiClient,
      officeApi,
      officeSeed.workspaceId,
      ["claude-acp"],
    );
    await officeApi.updateRouting(officeSeed.workspaceId, { ...configured, enabled: false });

    const routing = await officeApi.getRouting(officeSeed.workspaceId);
    expect(routing.config.enabled).toBe(false);

    await testPage.goto("/office/workspace/routing");
    await expect(
      testPage.getByRole("heading", { name: "Provider routing", exact: true }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "provider routing editor");

    const profileSelector = testPage.locator('[data-testid^="tier-profile-"]').first();
    await expect(profileSelector).toBeVisible();
    await profileSelector.click();
    await expect(testPage.getByRole("option").first()).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "provider routing profile selector");
  });
});
