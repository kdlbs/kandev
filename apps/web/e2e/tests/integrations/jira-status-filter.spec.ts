import { test, expect } from "../../fixtures/test-base";

// Covers the ticket-list Status filter (issue #1588): the filter must offer the
// selected project's real workflow statuses, not the three coarse status
// categories, and must be disabled until a project is selected.
test.describe("Jira status filter", () => {
  const PROJECT_KEY = "CLIP";

  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockJiraReset();
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");

    await apiClient.mockJiraSetProjects([{ id: "1", key: PROJECT_KEY, name: "Clip" }]);
    await apiClient.mockJiraSetProjectStatuses(PROJECT_KEY, [
      { id: "10001", name: "In Development", statusCategory: "indeterminate" },
      { id: "10002", name: "Ready for review", statusCategory: "indeterminate" },
    ]);
    // The project pill lists only projects the user has tickets in, derived from
    // the assignee/reporter search hits — so the seeded tickets double as both
    // that discovery source and the list the status filter narrows.
    await apiClient.mockJiraSetSearchHits([
      {
        key: "CLIP-1",
        summary: "Dev ticket",
        projectKey: PROJECT_KEY,
        statusName: "In Development",
        statusCategory: "indeterminate",
        url: "https://acme.atlassian.net/browse/CLIP-1",
      },
      {
        key: "CLIP-2",
        summary: "Review ticket",
        projectKey: PROJECT_KEY,
        statusName: "Ready for review",
        statusCategory: "indeterminate",
        url: "https://acme.atlassian.net/browse/CLIP-2",
      },
    ]);
  });

  test("status filter is disabled until a project is selected", async ({ testPage }) => {
    await testPage.goto("/jira");
    const statusPill = testPage.getByTestId("jira-filter-pill-status");
    await expect(statusPill).toBeVisible();
    await expect(statusPill).toHaveAttribute("data-disabled", "true");
  });

  test("selecting a project lists its real statuses and narrows the list", async ({ testPage }) => {
    await testPage.goto("/jira");

    // Pick the seeded project.
    await testPage.getByTestId("jira-filter-pill-project").click();
    await testPage.getByTestId(`jira-project-option-${PROJECT_KEY}`).click();
    await testPage.keyboard.press("Escape");

    // Status pill is now enabled and offers the project's real statuses.
    const statusPill = testPage.getByTestId("jira-filter-pill-status");
    await expect(statusPill).not.toHaveAttribute("data-disabled", "true");
    await statusPill.click();
    await expect(testPage.getByTestId("jira-status-option-In Development")).toBeVisible();
    await expect(testPage.getByTestId("jira-status-option-Ready for review")).toBeVisible();
    // The old coarse categories must NOT appear.
    await expect(testPage.getByTestId("jira-status-option-To Do")).toHaveCount(0);
    await expect(testPage.getByTestId("jira-status-option-In Progress")).toHaveCount(0);

    // Both tickets show before narrowing.
    await expect(testPage.getByText("CLIP-1")).toBeVisible();
    await expect(testPage.getByText("CLIP-2")).toBeVisible();

    // Checking "Ready for review" narrows to the matching ticket only.
    await testPage.getByTestId("jira-status-option-Ready for review").click();
    await expect(testPage.getByText("CLIP-2")).toBeVisible();
    await expect(testPage.getByText("CLIP-1")).toHaveCount(0);
  });

  test("confirms deletion of a custom view while preserving built-ins", async ({
    testPage,
    apiClient,
  }) => {
    const seedResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      jira_saved_views: [
        {
          id: "jira-sprint-bugs",
          name: "Sprint bugs",
          filters: {
            projectKeys: [],
            statuses: [],
            assignee: "anyone",
            searchText: "",
            sort: "updated",
          },
        },
      ],
    });
    expect(seedResponse.ok).toBe(true);
    await testPage.goto("/jira");

    await testPage.getByRole("button", { name: "Assigned to me" }).click();
    await expect(testPage.getByRole("button", { name: "Delete Assigned to me" })).toHaveCount(0);
    const deleteAction = testPage.getByRole("button", { name: "Delete Sprint bugs" });
    await deleteAction.click();
    const confirmation = testPage.getByTestId("saved-task-view-delete-confirmation");
    await expect(confirmation).toHaveAccessibleName("Delete Sprint bugs?");
    await expect(testPage.getByText("Sprint bugs", { exact: true })).toBeVisible();
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    const settingsAfterCancel = await apiClient.getUserSettings();
    expect(
      (settingsAfterCancel.settings.jira_saved_views as Array<{ id?: string }> | undefined)?.some(
        (view) => view.id === "jira-sprint-bugs",
      ),
    ).toBe(true);

    await deleteAction.click();
    const deleteResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await confirmation.getByRole("button", { name: "Delete Sprint bugs" }).click();
    await deleteResponse;
    await expect(deleteAction).toHaveCount(0);
  });
});
