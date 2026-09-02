import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { GITLAB_PROJECT } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";
import { ISSUES_ENDPOINT, seededIssue } from "./gitlab-issue-milestone-filter-helpers";

test.describe("GitLab issue milestone filter", () => {
  test("composes with a preset, narrows the list, and clears on preset/kind switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabAddIssues(seedData.workspaceId, GITLAB_PROJECT, [
      seededIssue(501, "Ship OAuth callback", { milestone: "Next" }),
      seededIssue(502, "Improve onboarding copy"),
    ]);

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    const scopeBar = testPage.getByTestId("gitlab-presets-scope-bar");
    const initialIssuesLoaded = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();
    await initialIssuesLoaded;

    await expect(gitlab.issueRow(501)).toBeVisible();
    await expect(gitlab.issueRow(502)).toBeVisible();
    // The chip reflects the issue's own milestone regardless of the filter.
    await expect(gitlab.issueRow(501).getByTestId("gitlab-issue-milestone")).toHaveText("Next");
    await expect(gitlab.issueRow(502).getByTestId("gitlab-issue-milestone")).toHaveCount(0);

    const milestoneInput = testPage.getByTestId("gitlab-milestone-filter");
    await expect(milestoneInput).toBeVisible();

    // Typing without committing issues no request: both rows stay put.
    await milestoneInput.fill("Next");
    await expect(gitlab.issueRow(502)).toBeVisible();

    const narrowed = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.press("Enter");
    await narrowed;
    await expect(gitlab.issueRow(501)).toBeVisible();
    await expect(gitlab.issueRow(502)).toHaveCount(0);

    // No match renders the empty state, not an error.
    const noMatch = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("Nonexistent");
    await milestoneInput.press("Enter");
    await noMatch;
    await expect(testPage.getByText("No issues match this filter.", { exact: true })).toBeVisible();

    // Clearing the milestone restores the unfiltered list.
    const cleared = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("");
    await milestoneInput.press("Enter");
    await cleared;
    await expect(gitlab.issueRow(501)).toBeVisible();
    await expect(gitlab.issueRow(502)).toBeVisible();

    const reNarrowed = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("Next");
    await milestoneInput.press("Enter");
    await reNarrowed;
    await expect(gitlab.issueRow(502)).toHaveCount(0);

    // Switching the sidebar preset clears the committed milestone.
    const presetSwitched = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await scopeBar.getByRole("button", { name: "Created", exact: true }).click();
    await presetSwitched;
    await expect(milestoneInput).toHaveValue("");
    await expect(gitlab.issueRow(502)).toBeVisible();

    // Merge requests view never renders the milestone control and is unaffected.
    await scopeBar.getByRole("button", { name: "Merge requests", exact: true }).click();
    await expect(testPage.getByTestId("gitlab-milestone-filter")).toHaveCount(0);

    // Switching back to Issues shows an empty input over an unnarrowed list.
    const backToIssues = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();
    await backToIssues;
    await expect(testPage.getByTestId("gitlab-milestone-filter")).toHaveValue("");
    await expect(gitlab.issueRow(501)).toBeVisible();
    await expect(gitlab.issueRow(502)).toBeVisible();
  });

  test("trims the committed milestone, saves a query, and restores it on reselect", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // This test chains many sequential network round-trips (commit, save,
    // preset switch, reselect, delete); give it headroom beyond the 60s
    // project default rather than relying on causal waits to also outrun
    // per-step host-load variance.
    test.setTimeout(180_000);
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabAddIssues(seedData.workspaceId, GITLAB_PROJECT, [
      seededIssue(601, "Milestone-scoped issue", { milestone: "Sprint 42" }),
      seededIssue(602, "Unrelated issue"),
    ]);

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    const scopeBar = testPage.getByTestId("gitlab-presets-scope-bar");
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();

    const milestoneInput = testPage.getByTestId("gitlab-milestone-filter");
    const committed = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("  Sprint 42  ");
    await milestoneInput.press("Enter");
    await committed;
    // Commit normalizes the visible input, not just the outgoing request.
    await expect(milestoneInput).toHaveValue("Sprint 42");
    await expect(gitlab.issueRow(601)).toBeVisible();
    await expect(gitlab.issueRow(602)).toHaveCount(0);

    const savedMenu = testPage.getByTestId("gitlab-saved-queries-menu");
    await savedMenu.click();
    const saveItem = testPage.getByRole("menuitem", { name: "Save current query" });
    await expect(saveItem).toBeEnabled();
    await saveItem.click();

    const dialog = testPage.getByRole("dialog", { name: "Save GitLab query" });
    await expect(dialog).toBeVisible();
    // Suggested-label precedence: milestone alone, since query and project are empty.
    await expect(dialog.getByLabel("Name")).toHaveValue("Sprint 42");
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(savedMenu).toContainText("Sprint 42");

    // Selecting a sidebar preset clears the milestone and widens the list again.
    const presetSwitched = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await scopeBar.getByRole("button", { name: "Created", exact: true }).click();
    await presetSwitched;
    await expect(milestoneInput).toHaveValue("");
    await expect(gitlab.issueRow(602)).toBeVisible();

    // Reselecting the saved query restores its committed milestone.
    await savedMenu.click();
    const reselected = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await testPage.getByRole("menuitem", { name: "Sprint 42", exact: true }).click();
    await reselected;
    await expect(milestoneInput).toHaveValue("Sprint 42");
    await expect(gitlab.issueRow(601)).toBeVisible();
    await expect(gitlab.issueRow(602)).toHaveCount(0);

    // Selecting the item above closes the dropdown via Radix's own
    // data-closed:animate-out exit transition (duration-100 in
    // @kandev/ui/dropdown-menu.tsx); Radix keeps the outgoing menu instance
    // mounted for that ~100ms while it animates out. Reopening the trigger
    // before that finishes races the old instance's unmount and can leave
    // the freshly reopened menu without content (root cause of this test's
    // documented pre-existing flake — confirmed via a CI failure artifact
    // showing the menu closed, not mid-click, at time of failure). Waiting
    // for the menu to fully close first removes the race.
    await expect(testPage.getByRole("menu")).toBeHidden();

    // Deleting the selected saved query clears the milestone in the same update.
    await savedMenu.click();
    const deleted = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await testPage.getByRole("menuitem", { name: "Delete Sprint 42 saved query" }).click();
    await deleted;
    await expect(milestoneInput).toHaveValue("");
    await expect(gitlab.issueRow(602)).toBeVisible();
  });

  test("resets pagination to page 1 when the committed milestone changes from page 2 (Scenario 6, clause 1)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // All 30 issues share one milestone, so filtering by it does not shrink the
    // result count below the 25-per-page threshold: the page still resets even
    // though the total (and therefore totalPages) is unchanged by the commit.
    const issues = Array.from({ length: 30 }, (_, i) =>
      seededIssue(801 + i, `Milestone issue ${i + 1}`, { milestone: "Sprint 1" }),
    );
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabAddIssues(seedData.workspaceId, GITLAB_PROJECT, issues);

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    const scopeBar = testPage.getByTestId("gitlab-presets-scope-bar");
    const initialIssuesLoaded = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();
    await initialIssuesLoaded;
    await expect(gitlab.issueRow(801)).toBeVisible();

    const page2Loaded = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await testPage.getByRole("link", { name: "2", exact: true }).click();
    await page2Loaded;
    await expect(gitlab.issueRow(830)).toBeVisible();
    await expect(testPage.getByRole("link", { name: "2", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );

    const milestoneInput = testPage.getByTestId("gitlab-milestone-filter");
    const filtered = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("Sprint 1");
    await milestoneInput.press("Enter");
    await filtered;

    await expect(gitlab.issueRow(801)).toBeVisible();
    await expect(testPage.getByRole("link", { name: "1", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});
