import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GITLAB_PROJECT } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";
import { ISSUES_ENDPOINT, seededIssue } from "./gitlab-issue-milestone-filter-helpers";

test.describe("Mobile GitLab issue milestone filter", () => {
  test("filters issues from the mobile sheet without clipping the milestone input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabAddIssues(seedData.workspaceId, GITLAB_PROJECT, [
      seededIssue(901, "Mobile milestone issue", { milestone: "Next" }),
      seededIssue(902, "Mobile unrelated issue"),
    ]);

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    await gitlab.mobileFiltersButton.tap();
    await expect(gitlab.mobileSidebar).toBeVisible();

    const issuesLoaded = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await gitlab.mobileSidebar.getByTestId("gitlab-kind-issue").tap();
    await issuesLoaded;
    await expect(gitlab.mobileSidebar).toBeHidden();

    const milestoneInput = testPage.getByTestId("gitlab-milestone-filter");
    await expect(milestoneInput).toBeVisible();
    const inputBox = await milestoneInput.boundingBox();
    expect(inputBox, "mobile milestone input has no bounding box").not.toBeNull();
    if (inputBox) expect(Math.round(inputBox.height)).toBeGreaterThanOrEqual(44);

    const filtered = waitForHttp(testPage, "GET", ISSUES_ENDPOINT);
    await milestoneInput.fill("Next");
    await milestoneInput.press("Enter");
    await filtered;
    await expect(gitlab.issueRow(901)).toBeVisible();
    await expect(gitlab.issueRow(902)).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile GitLab issue milestone filter");
  });
});
