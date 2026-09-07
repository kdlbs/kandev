import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GITLAB_PROJECT } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";
import { ISSUES_ENDPOINT, seededIssue } from "./gitlab-issue-milestone-filter-helpers";

test.describe("Mobile GitLab issue milestone filter", () => {
  test("confirms a saved query inline with visible touch actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const savedLabel = "Mobile release MRs";
    await apiClient.configureGitLab(seedData.workspaceId);
    const seedResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      gitlab_saved_presets: [
        {
          id: "mobile-release-mrs",
          kind: "mr",
          label: savedLabel,
          customQuery: "state=opened",
          projectFilter: GITLAB_PROJECT,
          milestone: "",
          preset: "",
          createdAt: "2026-09-07T00:00:00Z",
        },
      ],
    });
    expect(seedResponse.ok).toBe(true);

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    await gitlab.mobileFiltersButton.tap();
    const deleteAction = gitlab.mobileSidebar.getByRole("button", {
      name: `Delete ${savedLabel} saved query`,
      exact: true,
    });
    await expect(deleteAction).toBeVisible();
    const deleteBox = await deleteAction.boundingBox();
    expect(deleteBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(deleteBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    await deleteAction.tap();

    const confirmation = gitlab.mobileSidebar.getByTestId("saved-task-view-delete-confirmation");
    await expect(confirmation).toHaveAccessibleName(`Delete ${savedLabel}?`);
    await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(deleteAction).toBeVisible();

    await deleteAction.tap();
    const deleteResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await gitlab.mobileSidebar
      .getByTestId("saved-task-view-delete-confirmation")
      .getByRole("button", { name: `Delete ${savedLabel}` })
      .tap();
    await deleteResponse;
    await expect(deleteAction).toHaveCount(0);
    await expect(gitlab.mobileSidebar).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile GitLab saved query deletion");
  });

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
