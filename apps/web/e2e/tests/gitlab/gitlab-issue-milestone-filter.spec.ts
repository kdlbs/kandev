import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { MockGitLabIssueSeed } from "../../helpers/api-client";
import { GITLAB_PROJECT } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";

const ISSUES_ENDPOINT = /^\/api\/v1\/gitlab\/user\/issues$/;

function seededIssue(
  iid: number,
  title: string,
  overrides: { milestone?: string } = {},
): MockGitLabIssueSeed {
  const now = new Date().toISOString();
  return {
    id: iid + 20_000,
    iid,
    project_id: 101,
    title,
    body: `Body for ${title}`,
    url: `https://gitlab.example.test/${GITLAB_PROJECT}/-/issues/${iid}`,
    web_url: `https://gitlab.example.test/${GITLAB_PROJECT}/-/issues/${iid}`,
    state: "opened",
    author_username: "reporter",
    project_namespace: "platform",
    project_path: GITLAB_PROJECT,
    labels: [],
    assignees: [],
    milestone: overrides.milestone ?? "",
    created_at: now,
    updated_at: now,
  };
}

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
    // Retained at its pre-existing value: the delete step below keeps its
    // pre-existing bounded-timeout click (see comment there) rather than an
    // armed wait, so this budget still needs to absorb that step's documented
    // host-load variance, not just the causal-waited steps above it.
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

    // Deleting the selected saved query clears the milestone in the same update.
    // Unlike every other step above, this one keeps its pre-existing bounded
    // timeout instead of an armed waitForHttp: the click target is the menu
    // item being deleted, so it can detach from the DOM mid-click under load,
    // and racing that against a concurrently-pending network wait risks an
    // unhandled rejection if the click's own actionability retries outlast
    // the wait's budget. This exact interaction is the test's documented,
    // independently-investigated pre-existing flake (see task plan); the
    // trailing assertions' own polling plus this timeout is the established
    // mitigation, not a new one.
    await savedMenu.click();
    await testPage.getByRole("menuitem", { name: "Delete Sprint 42 saved query" }).click();
    await expect(milestoneInput).toHaveValue("");
    await expect(gitlab.issueRow(602)).toBeVisible({ timeout: 20_000 });
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
