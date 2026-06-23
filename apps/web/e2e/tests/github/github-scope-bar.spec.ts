import { test, expect } from "../../fixtures/test-base";

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
    const search = testPage.getByTestId("github-repo-filter-search").getByRole("combobox");
    await expect(search).toBeVisible();

    await search.fill("testorg");
    await testPage.getByRole("option", { name: "testorg/testrepo" }).click();
    await expect(testPage.getByTestId("github-repo-filter-trigger")).toContainText(
      "testorg/testrepo",
    );
  });
});
