import { test, expect } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

test.describe("Office execution profile routing on mobile", () => {
  test("keeps the first-provider profile selector usable with routing disabled", async ({
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

  test("edits and saves a role tier on mobile", async ({
    backend,
    apiClient,
    officeApi,
    officeSeed,
    testPage,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
    const configured = await balancedExecutionProfileRouting(
      apiClient,
      officeApi,
      officeSeed.workspaceId,
      ["claude-acp"],
    );
    await officeApi.updateRouting(officeSeed.workspaceId, { ...configured, enabled: false });

    await testPage.goto("/office/workspace/routing");
    const roleTier = testPage.getByRole("combobox", { name: "CEO", exact: true });
    await expect(roleTier).toBeVisible();
    const roleTierBox = await roleTier.boundingBox();
    expect(roleTierBox?.width).toBeGreaterThanOrEqual(44);
    expect(roleTierBox?.height).toBeGreaterThanOrEqual(44);

    const infoButton = testPage.getByRole("button", { name: "More info about CEO", exact: true });
    const infoButtonBox = await infoButton.boundingBox();
    expect(infoButtonBox?.width).toBeGreaterThanOrEqual(44);
    expect(infoButtonBox?.height).toBeGreaterThanOrEqual(44);

    await roleTier.click();
    await testPage.getByRole("option", { name: /Balanced/ }).click();
    await testPage.getByRole("button", { name: "Save", exact: true }).click();

    const saved = await officeApi.getRouting(officeSeed.workspaceId);
    expect(saved.config.role_tiers?.ceo).toBe("balanced");

    await testPage.reload();
    await expect(testPage.getByRole("combobox", { name: "CEO", exact: true })).toContainText(
      "Balanced",
    );
    await assertNoDocumentHorizontalOverflow(testPage, "role tier editor");
  });
});
