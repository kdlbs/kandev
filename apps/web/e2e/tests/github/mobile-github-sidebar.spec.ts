import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { expectContentSizedBottomConfirmation } from "../../helpers/mobile-confirmations";

const DEFAULT_REPO = "mobileorg/defaultrepo";
const DEFAULT_REPO_ISSUE = "Mobile issue in saved default repo";
const OTHER_REPO_ISSUE = "Mobile issue outside saved default repo";
const SAVED_QUERY_FILTER = "assignee:@me is:open mobile-saved-default-repo-e2e";
const DEFAULTS_TIMESTAMP = "2026-08-07T00:00:00Z";

test.describe("Mobile /github sidebar", () => {
  // @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
  test("opens the save form after the Views drawer releases focus", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await github.mobileMenuButton.tap();
    await expect(github.mobileSidebar).toBeVisible();
    await testPage.addStyleTag({
      content:
        '[data-testid="github-mobile-sidebar"][data-state="closed"] { animation-duration: 1s !important; transition-duration: 1s !important; }',
    });
    await github.mobileSidebar.getByRole("button", { name: "Save current query" }).tap();
    const dialog = testPage.getByRole("dialog", { name: "Save query", exact: true });
    await expect(dialog).toBeVisible();
    expect(await github.mobileSidebar.isVisible()).toBe(false);
    await dialog.getByLabel("Name", { exact: true }).fill("Keep this draft");
    await expect(dialog.getByLabel("Name", { exact: true })).toBeFocused();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(dialog).toBeHidden();
    await expect(github.mobileMenuButton).toBeFocused();
  });

  // @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.2
  test("keeps the view picker open while changing result kind", async ({ testPage, apiClient }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
    await expect(github.mobileSidebar).toBeVisible();
    await github.presetByLabel("Mentions").tap();
    await expect(github.mobileSidebar).toBeHidden();
    await expect(github.toolbarTitle).toContainText("Mentions");
    await expect(github.mobileMenuButton).toBeFocused();
  });

  // @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
  test("retains a failed save for retry", async ({ testPage, apiClient, seedData }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const baseline = await testPage.request.get("/api/v1/github/workspace-settings", {
      params: { workspace_id: seedData.workspaceId },
    });
    expect(baseline.ok()).toBe(true);
    const settings = await baseline.json();
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Save current query" }).tap();
    const dialog = testPage.getByRole("dialog", { name: "Save query" });
    await dialog.getByLabel("Name", { exact: true }).fill("Retry my view");
    let failSave = true;
    await testPage.route("**/api/v1/github/workspace-settings", async (route) => {
      if (route.request().method() === "PUT" && failSave) {
        failSave = false;
        await route.fulfill({ status: 503, json: { error: "settings unavailable" } });
      } else await route.continue();
    });
    try {
      const failed = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/github/workspace-settings") && response.status() === 503,
      );
      await dialog.getByRole("button", { name: "Save", exact: true }).tap();
      await failed;
      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Retry my view");
      const saved = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/github/workspace-settings") &&
          response.request().method() === "PUT" &&
          response.ok(),
      );
      await dialog.getByRole("button", { name: "Save", exact: true }).tap();
      await saved;
      await expect(dialog).toBeHidden();
      await expect(github.toolbarTitle).toHaveText("Retry my view");
    } finally {
      await testPage.unroute("**/api/v1/github/workspace-settings");
      const restored = await testPage.request.put("/api/v1/github/workspace-settings", {
        data: { workspace_id: seedData.workspaceId, saved_presets: settings.saved_presets ?? [] },
      });
      expect(restored.ok()).toBe(true);
    }
  });

  test.describe("saved default views", () => {
    test.afterEach(async ({ testPage, seedData }) => {
      const cleanupResponse = await testPage.request.put("/api/v1/github/workspace-settings", {
        data: { workspace_id: seedData.workspaceId, saved_presets: [] },
      });
      expect(cleanupResponse.ok(), "afterEach saved-presets cleanup").toBe(true);
    });

    test("saved default action is touch-sized, isolated, and persistent", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const savedLabel = "Mobile future issues";
      await apiClient.mockGitHubReset();
      await apiClient.mockGitHubSetUser("test-user");
      await apiClient.mockGitHubAddIssues([
        {
          number: 190,
          title: DEFAULT_REPO_ISSUE,
          state: "open",
          author_login: "test-user",
          repo_owner: "mobileorg",
          repo_name: "defaultrepo",
          assignees: ["test-user"],
        },
      ]);
      const seedResponse = await testPage.request.put("/api/v1/github/workspace-settings", {
        data: {
          workspace_id: seedData.workspaceId,
          saved_presets: [
            {
              id: "mobile-saved-issue",
              kind: "issue",
              label: savedLabel,
              customQuery: "assignee:@me is:open",
              repoFilter: DEFAULT_REPO,
              createdAt: DEFAULTS_TIMESTAMP,
              isDefault: false,
            },
          ],
        },
      });
      expect(seedResponse.ok()).toBe(true);

      const page = new MobileGitHubPage(testPage);
      await page.goto();
      await page.mobileMenuButton.tap();
      await page.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
      await expect(page.toolbarTitle).toContainText("Assigned");

      const defaultAction = page.savedQueryDefaultAction(savedLabel);
      const actionBox = await defaultAction.boundingBox();
      expect(actionBox).not.toBeNull();
      expect(actionBox!.height).toBeGreaterThanOrEqual(44);
      expect(actionBox!.width).toBeGreaterThanOrEqual(44);
      const setResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/github/workspace-settings") &&
          response.request().method() === "PUT" &&
          response.status() === 200,
      );
      await defaultAction.tap();
      await setResponse;

      await expect(page.mobileSidebar).toBeVisible();
      await expect(page.toolbarTitle).toContainText("Assigned");
      await expect(page.savedQueryDefaultAction(savedLabel, "Clear")).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      await testPage.reload();
      await page.mobileMenuButton.waitFor({ state: "visible" });
      await page.mobileMenuButton.tap();
      await page.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
      await expect(page.toolbarTitle).toContainText(savedLabel);
      await expect(page.repoFilterTrigger).toContainText(DEFAULT_REPO);
    });
  });

  test("Views opens the drawer and selecting a preset updates the toolbar", async ({
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
    // named Views button is visible.
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
    prCapture,
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
    await page.mobileSidebar.getByRole("button", { name: "Done", exact: true }).tap();
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
    await page.savedQueryByLabel(savedQuery).tap();
    await expect(page.repoFilterTrigger).toContainText(DEFAULT_REPO);
    await expect(page.issueRowByTitle(DEFAULT_REPO_ISSUE)).toBeVisible();
    await expect(page.issueRowByTitle(OTHER_REPO_ISSUE)).toHaveCount(0);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await page.mobileMenuButton.tap();
    const deleteAction = page.mobileSidebar.getByRole("button", {
      name: `Delete ${savedQuery} saved query`,
    });
    await waitForFiniteAnimations(page.mobileSidebar);
    const deleteBox = await deleteAction.boundingBox();
    expect(deleteBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(deleteBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    const originalDialogId = await page.mobileSidebar.getAttribute("id");
    await expect(page.mobileSidebar).toHaveAttribute("data-vaul-drawer-direction", "bottom");
    await deleteAction.tap();
    const deleteConfirmation = page.mobileSidebar.getByTestId(
      "saved-task-view-delete-confirmation",
    );
    await expect(deleteConfirmation).toHaveAccessibleName(`Delete ${savedQuery}?`);
    await expectContentSizedBottomConfirmation(page.mobileSidebar, deleteConfirmation);
    await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
    await expect(page.mobileSidebar).toHaveAttribute("id", originalDialogId!);
    await expect(deleteConfirmation.getByRole("button", { name: "Back" })).toBeVisible();
    await waitForFiniteAnimations(page.mobileSidebar);
    await prCapture.screenshot("saved-query-delete-mobile", {
      caption: "A saved integration query becomes a confirmation step inside the filter sheet.",
    });
    await deleteConfirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(page.savedQueryByLabel(savedQuery)).toBeVisible();

    await deleteAction.tap();
    const deleteResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PUT" &&
        response.url().includes("/api/v1/github/workspace-settings"),
    );
    await page.mobileSidebar
      .getByTestId("saved-task-view-delete-confirmation")
      .getByRole("button", { name: `Delete ${savedQuery}` })
      .tap();
    await deleteResponse;
    await expect(page.savedQueryByLabel(savedQuery)).toHaveCount(0);
    await expect(page.mobileSidebar).toBeVisible();
  });
});
