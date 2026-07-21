import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

const DEFAULT_REPO = "defaultorg/defaultrepo";
const DEFAULT_REPO_ISSUE = "Issue in saved default repo";
const OTHER_REPO_ISSUE = "Issue outside saved default repo";
const SAVED_QUERY_FILTER = "assignee:@me is:open saved-default-repo-e2e";

function issueRow(testPage: Page, title: string) {
  return testPage.getByTestId("issue-row").filter({ hasText: title });
}

// The /github dashboard used to render its own w-60 presets rail next to the
// global AppSidebar (a redundant "double sidebar"). On desktop that rail is now
// a horizontal scope bar; this guards that it drives presets/kind and that the
// old inline rail is gone. Mobile keeps the sheet (see mobile-github-sidebar).
test.describe("Desktop /github scope bar", () => {
  test("scope bar drives presets and kind, with no second sidebar", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 11,
        title: "Scope bar PR",
        state: "open",
        head_branch: "feat/scope",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    await testPage.goto("/github");

    const scopeBar = testPage.getByTestId("github-presets-scope-bar");
    const title = testPage.getByTestId("github-list-toolbar-title");

    // The horizontal scope bar is present; the old inline rail is gone.
    await expect(scopeBar).toBeVisible();
    await expect(testPage.getByTestId("github-presets-sidebar-inline")).toHaveCount(0);

    // Default PR preset is active.
    await expect(title).toContainText("Review requested");

    // Clicking a preset pill updates the active query.
    await scopeBar.getByRole("button", { name: "Mentions" }).click();
    await expect(title).toContainText("Mentions");

    // Switching kind to Issues falls back to the first issue preset.
    await scopeBar.getByRole("button", { name: "Issues" }).click();
    await expect(title).toContainText("Assigned");
  });

  test("repo filter menu can be searched", async ({ testPage, apiClient }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 21,
        title: "Searchable repo filter PR",
        state: "open",
        head_branch: "feat/search",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 22,
        title: "Searchable repo filter second PR",
        state: "open",
        head_branch: "feat/other",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "anotherorg",
        repo_name: "secondrepo",
      },
    ]);

    await testPage.goto("/github");

    await testPage.getByTestId("github-repo-filter-trigger").click();
    const search = testPage.getByTestId("github-repo-filter-dropdown").getByRole("combobox");
    await expect(search).toBeVisible();

    await search.fill("testorg");
    await testPage.getByRole("option", { name: "testorg/testrepo" }).click();
    await expect(testPage.getByTestId("github-repo-filter-trigger")).toContainText(
      "testorg/testrepo",
    );
  });

  test("saved query defaults to its chosen repository and persists", async ({
    testPage,
    apiClient,
  }) => {
    const savedQuery = "Default repo issues";
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddIssues([
      {
        number: 31,
        title: DEFAULT_REPO_ISSUE,
        state: "open",
        author_login: "test-user",
        repo_owner: "defaultorg",
        repo_name: "defaultrepo",
        assignees: ["test-user"],
      },
      {
        number: 32,
        title: OTHER_REPO_ISSUE,
        state: "open",
        author_login: "test-user",
        repo_owner: "otherorg",
        repo_name: "otherrepo",
        assignees: ["test-user"],
      },
    ]);

    await testPage.goto("/github");

    const scopeBar = testPage.getByTestId("github-presets-scope-bar");
    const savedMenu = testPage.getByTestId("github-saved-queries-menu");
    const repoFilter = testPage.getByTestId("github-repo-filter-trigger");
    await expect(scopeBar).toBeVisible();
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();
    const queryInput = testPage.getByPlaceholder(/Custom query/);
    await queryInput.fill(SAVED_QUERY_FILTER);
    await queryInput.press("Enter");
    await expect(testPage.getByTestId("issue-row")).toHaveCount(2, { timeout: 15_000 });

    await savedMenu.click();
    await testPage.getByRole("menuitem", { name: "Save current query" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Save query" });
    await dialog.getByLabel("Name").fill(savedQuery);
    const saveRepoTrigger = dialog.getByTestId("github-save-query-repo-trigger");
    await expect(saveRepoTrigger).toBeVisible();
    await expect
      .poll(async () => (await saveRepoTrigger.boundingBox())?.height ?? 0)
      .toBeCloseTo(36, 0);
    await saveRepoTrigger.click();
    const saveRepoDropdown = testPage.getByTestId("github-save-query-repo-dropdown");
    await expect(saveRepoDropdown).toBeVisible();
    await saveRepoDropdown.getByRole("option", { name: DEFAULT_REPO, exact: true }).click();
    const saveResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/github/workspace-settings") &&
        response.request().method() === "PUT" &&
        response.status() === 200,
    );
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await saveResponse;

    await expect(repoFilter).toContainText(DEFAULT_REPO);
    await expect(issueRow(testPage, DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(issueRow(testPage, OTHER_REPO_ISSUE)).toHaveCount(0);

    await repoFilter.click();
    await testPage
      .getByTestId("github-repo-filter-dropdown")
      .getByRole("option", { name: "All repos", exact: true })
      .click();
    await expect(testPage.getByTestId("issue-row")).toHaveCount(2);

    await savedMenu.click();
    await testPage.getByRole("menuitem").filter({ hasText: savedQuery }).click();
    await expect(repoFilter).toContainText(DEFAULT_REPO);
    await expect(issueRow(testPage, DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(issueRow(testPage, OTHER_REPO_ISSUE)).toHaveCount(0);

    await testPage.reload();
    await expect(scopeBar).toBeVisible();
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();
    await savedMenu.click();
    await testPage.getByRole("menuitem").filter({ hasText: savedQuery }).click();
    await expect(repoFilter).toContainText(DEFAULT_REPO);
    await expect(issueRow(testPage, DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(issueRow(testPage, OTHER_REPO_ISSUE)).toHaveCount(0);
  });
});
