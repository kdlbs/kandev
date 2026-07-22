import { test, expect } from "../../fixtures/test-base";
import { GITLAB_HOST, GITLAB_PROJECT, gitLabMR, seedGitLabReview } from "../../helpers/gitlab";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { GitLabPage } from "../../pages/gitlab-page";

test.describe("GitLab workspace parity", () => {
  test("keeps self-managed browse results isolated when switching workspaces", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("GitLab Isolated Workspace");
    const otherHost = "https://gitlab.other.test";
    try {
      await seedGitLabReview(apiClient, seedData.workspaceId, 71, "Primary workspace MR");
      await apiClient.configureGitLab(other.id, otherHost);
      await apiClient.mockGitLabAddMRs(other.id, GITLAB_PROJECT, [
        gitLabMR(72, "Other workspace MR", {
          url: `${otherHost}/${GITLAB_PROJECT}/-/merge_requests/72`,
          web_url: `${otherHost}/${GITLAB_PROJECT}/-/merge_requests/72`,
        }),
      ]);

      const gitlab = new GitLabPage(testPage);
      await gitlab.goto();
      await expect(gitlab.mrRow(71)).toContainText("Primary workspace MR");
      await expect(gitlab.mrRow(72)).toHaveCount(0);
      await expect(testPage.getByText(GITLAB_HOST, { exact: false })).toBeVisible();
      await assertLocatorWithinViewportX(gitlab.mrRow(71), "primary GitLab MR row");
      await assertNoDocumentHorizontalOverflow(testPage, "primary GitLab workspace");

      await testPage.getByTestId("sidebar-workspace-trigger").click();
      await testPage.getByTestId(`sidebar-workspace-item-${other.id}`).click();
      await expect(testPage).toHaveURL(new RegExp(`workspaceId=${other.id}`));
      await gitlab.goto();
      await expect(gitlab.mrRow(72)).toContainText("Other workspace MR");
      await expect(gitlab.mrRow(71)).toHaveCount(0);
      await expect(testPage.getByText(otherHost, { exact: false })).toBeVisible();
      await assertLocatorWithinViewportX(gitlab.mrRow(72), "isolated GitLab MR row");
      await assertNoDocumentHorizontalOverflow(testPage, "isolated GitLab workspace");
    } finally {
      await apiClient.mockGitLabReset(other.id).catch(() => undefined);
      await apiClient
        .rawRequest("DELETE", `/api/v1/gitlab/config?workspace_id=${encodeURIComponent(other.id)}`)
        .catch(() => undefined);
      await apiClient.deleteWorkspace(other.id, other.name).catch(() => undefined);
    }
  });

  test("quick launches, reviews, updates, subscribes, and unlinks a merge request", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await seedGitLabReview(apiClient, seedData.workspaceId, 81, "Review GitLab parity");
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    await gitlab.startMRTask(81);
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "81");
    await gitlab.openLinkedMR(81);

    const panel = testPage.getByTestId("mr-detail-panel").last();
    await expect(panel.getByText("Review GitLab parity", { exact: true })).toBeVisible();
    await expect(panel.getByText("success · 4/4", { exact: true })).toBeVisible();
    await assertLocatorWithinViewportX(panel, "desktop GitLab review panel");
    await assertNoDocumentHorizontalOverflow(testPage, "desktop GitLab review");
    await prCapture.screenshot("desktop-merge-request-review", {
      caption: "GitLab merge request review with pipeline, reviewers, and discussion controls",
    });

    await panel.getByRole("button", { name: "Approve", exact: true }).click();
    await expect(testPage.getByText("Merge request approved", { exact: true })).toBeVisible();
    await expect(panel.getByText("1 approved", { exact: true })).toBeVisible();

    await panel.getByRole("button", { name: "Subscribe to GitLab notifications" }).click();
    await expect(
      testPage.getByText("Subscribed to GitLab notifications", { exact: true }),
    ).toBeVisible();
    await expect(
      panel.getByRole("button", { name: "Unsubscribe from GitLab notifications" }),
    ).toBeVisible();

    const reviewers = panel.getByRole("region", { name: "Reviewers" });
    await reviewers.getByRole("textbox", { name: "Search reviewers" }).fill("alice");
    await reviewers.getByRole("button", { name: "Search reviewers" }).click();
    await reviewers.getByText("Alice Reviewer", { exact: false }).click();
    await reviewers.getByRole("button", { name: "Apply" }).click();
    await expect(testPage.getByText("Reviewers updated", { exact: true })).toBeVisible();
    await expect(reviewers.getByText("alice", { exact: true })).toBeVisible();

    const discussion = panel.getByTestId("gitlab-discussion-thread-1");
    await discussion.getByRole("textbox", { name: "Discussion reply" }).fill("Added coverage.");
    await discussion.getByRole("button", { name: "Reply" }).click();
    await expect(discussion.getByText("Added coverage.", { exact: true })).toBeVisible();
    await discussion.getByRole("button", { name: "Resolve" }).click();
    await expect(discussion.getByText("Resolved", { exact: true })).toBeVisible();

    await panel.getByRole("button", { name: /^Files \(1\)$/ }).click();
    await expect(panel.getByText("src/main.ts", { exact: true })).toBeVisible();
    await panel.getByRole("button", { name: /^Commits \(1\)$/ }).click();
    await expect(panel.getByText("feat: add GitLab parity", { exact: true })).toBeVisible();

    await panel.getByRole("button", { name: "Unlink merge request" }).click();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveCount(0);

    await testPage.getByRole("button", { name: "Link GitLab merge request" }).click();
    const linkDialog = testPage.getByRole("dialog", { name: "Link GitLab merge request" });
    await linkDialog
      .getByLabel("Merge request URL")
      .fill(`${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/81`);
    await linkDialog.getByRole("button", { name: "Link merge request" }).click();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "81");

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "81");
    await gitlab.unlinkMR(81);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop GitLab manual link");
  });
});
