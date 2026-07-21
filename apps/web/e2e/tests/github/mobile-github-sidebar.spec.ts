import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";

const DEFAULT_REPO = "mobileorg/defaultrepo";
const DEFAULT_REPO_ISSUE = "Mobile issue in saved default repo";
const OTHER_REPO_ISSUE = "Mobile issue outside saved default repo";
const SAVED_QUERY_FILTER = "assignee:@me is:open mobile-saved-default-repo-e2e";

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

  test("saved query defaults to its chosen repository and persists by touch", async ({
    testPage,
    apiClient,
  }) => {
    const savedQuery = "Mobile default repo issues";
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddIssues([
      {
        number: 220,
        title: DEFAULT_REPO_ISSUE,
        state: "open",
        author_login: "test-user",
        repo_owner: "mobileorg",
        repo_name: "defaultrepo",
        assignees: ["test-user"],
      },
      {
        number: 221,
        title: OTHER_REPO_ISSUE,
        state: "open",
        author_login: "test-user",
        repo_owner: "otherorg",
        repo_name: "otherrepo",
        assignees: ["test-user"],
      },
    ]);

    const page = new MobileGitHubPage(testPage);
    await page.goto();
    await page.mobileMenuButton.tap();
    await page.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
    const queryInput = testPage.getByPlaceholder(/Custom query/);
    await queryInput.fill(SAVED_QUERY_FILTER);
    await queryInput.press("Enter");
    await expect(testPage.getByTestId("issue-row")).toHaveCount(2, { timeout: 15_000 });

    await page.mobileMenuButton.tap();
    await page.mobileSidebar.getByRole("button", { name: "Save current query" }).tap();
    const dialog = testPage.getByRole("dialog", { name: "Save query" });
    await dialog.getByLabel("Name").fill(savedQuery);
    await expect(page.saveQueryRepoTrigger).toBeVisible();
    await expect
      .poll(async () => (await page.saveQueryRepoTrigger.boundingBox())?.height ?? 0)
      .toBeCloseTo(44, 0);
    await page.saveQueryRepoTrigger.tap();
    await expect(page.saveQueryRepoDropdown).toBeVisible();
    await page.saveQueryRepoDropdown.getByRole("option", { name: DEFAULT_REPO, exact: true }).tap();
    const saveResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/github/workspace-settings") &&
        response.request().method() === "PUT" &&
        response.status() === 200,
    );
    await dialog.getByRole("button", { name: "Save", exact: true }).tap();
    await saveResponse;

    await expect(page.repoFilterTrigger).toContainText(DEFAULT_REPO);
    await expect(page.issueRowByTitle(DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(page.issueRowByTitle(OTHER_REPO_ISSUE)).toHaveCount(0);

    await page.repoFilterTrigger.tap();
    await testPage
      .getByTestId("github-repo-filter-dropdown")
      .getByRole("option", { name: "All repos", exact: true })
      .tap();
    await expect(testPage.getByTestId("issue-row")).toHaveCount(2);

    await page.mobileMenuButton.tap();
    await page.savedQueryByLabel(savedQuery).tap();
    await expect(page.repoFilterTrigger).toContainText(DEFAULT_REPO);
    await expect(page.issueRowByTitle(DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(page.issueRowByTitle(OTHER_REPO_ISSUE)).toHaveCount(0);

    await testPage.reload();
    await page.mobileMenuButton.waitFor({ state: "visible" });
    await page.mobileMenuButton.tap();
    await page.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
    await page.mobileMenuButton.tap();
    await page.savedQueryByLabel(savedQuery).tap();
    await expect(page.repoFilterTrigger).toContainText(DEFAULT_REPO);
    await expect(page.issueRowByTitle(DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(page.issueRowByTitle(OTHER_REPO_ISSUE)).toHaveCount(0);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
