import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";

test.describe("Mobile /github sidebar", () => {
  test("hamburger opens sheet and selecting a preset updates the toolbar", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 200,
        title: "Mobile sidebar PR",
        state: "open",
        head_branch: "feat/mobile",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const page = new MobileGitHubPage(testPage);
    await page.goto();

    // On a mobile viewport the inline desktop sidebar is hidden and the
    // hamburger menu button is visible.
    await expect(page.mobileMenuButton).toBeVisible();
    await expect(page.inlineSidebar).toBeHidden();

    // Default selection is the first PR preset.
    await expect(page.toolbarTitle).toContainText("Review requested");

    // Sheet is closed initially.
    await expect(page.mobileSidebar).toBeHidden();

    // Open the drawer.
    await page.mobileMenuButton.tap();
    await expect(page.mobileSidebar).toBeVisible();

    // Tap a different preset → the drawer closes and the toolbar title
    // reflects the new selection.
    await page.presetByLabel("Mentions").tap();
    await expect(page.mobileSidebar).toBeHidden();
    await expect(page.toolbarTitle).toContainText("Mentions");
  });

  test("repo filter menu can be searched", async ({ testPage, apiClient }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 210,
        title: "Mobile repo search PR",
        state: "open",
        head_branch: "feat/mobile-search",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 211,
        title: "Mobile repo search second PR",
        state: "open",
        head_branch: "feat/mobile-other",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "anotherorg",
        repo_name: "secondrepo",
      },
    ]);

    const page = new MobileGitHubPage(testPage);
    await page.goto();

    await page.repoFilterTrigger.tap();
    await expect(page.repoSearchInput).toBeVisible();

    await page.repoSearchInput.fill("testorg");
    await testPage.getByRole("option", { name: "testorg/testrepo" }).tap();
    await expect(page.repoFilterTrigger).toContainText("testorg/testrepo");
  });
});
