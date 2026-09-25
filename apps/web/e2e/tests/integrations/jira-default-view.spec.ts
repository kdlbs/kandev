import { expect, test } from "../../fixtures/test-base";

const PROJECT_KEY = "CLIP";
const VIEW_ID = "jira-default-open-tickets";
const VIEW_NAME = "My open tickets";
const CUSTOM_JQL = 'project = CLIP AND status = "In Development" ORDER BY updated DESC';

test.describe("Jira default view", () => {
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
    ]);
    await apiClient.mockJiraSetSearchHits([
      {
        key: "CLIP-1",
        summary: "Development ticket",
        projectKey: PROJECT_KEY,
        statusName: "In Development",
        statusCategory: "indeterminate",
        url: "https://acme.atlassian.net/browse/CLIP-1",
      },
    ]);
    const seedResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      jira_saved_views: [
        {
          id: VIEW_ID,
          name: VIEW_NAME,
          filters: {
            projectKeys: [PROJECT_KEY],
            statuses: ["In Development"],
            assignee: "anyone",
            searchText: "",
            sort: "updated",
          },
          customJql: CUSTOM_JQL,
        },
      ],
      jira_default_view_id: "",
    });
    expect(seedResponse.ok).toBe(true);
  });

  test("saves a custom JQL default, restores it on revisit, and falls back after deletion", async ({
    apiClient,
    testPage,
  }) => {
    await testPage.goto("/jira");
    const currentView = testPage
      .locator('button[aria-haspopup="dialog"]')
      .filter({ hasText: "Assigned to me" });
    await expect(currentView).toBeVisible();

    await currentView.click();
    await testPage.getByRole("button", { name: VIEW_NAME, exact: true }).hover();
    const setDefault = testPage.getByRole("button", {
      name: `Set ${VIEW_NAME} as default view`,
    });
    const saveDefault = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await setDefault.click();
    await saveDefault;

    // Setting a future default must leave the current result view untouched.
    await expect(currentView).toBeVisible();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.jira_default_view_id)
      .toBe(VIEW_ID);

    const customSearchRequest = testPage.waitForRequest((request) => {
      const url = new URL(request.url());
      return url.pathname === "/api/v1/jira/tickets" && url.searchParams.get("jql") === CUSTOM_JQL;
    });
    await testPage.goto("/jira");
    const restoredView = testPage
      .locator('button[aria-haspopup="dialog"]')
      .filter({ hasText: VIEW_NAME });
    await expect(restoredView).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator("textarea").first()).toHaveValue(CUSTOM_JQL);
    const searchUrl = new URL((await customSearchRequest).url());
    expect(searchUrl.searchParams.get("jql")).toBe(CUSTOM_JQL);
    await expect(testPage.getByText("CLIP-1")).toBeVisible();

    await restoredView.click();
    const deleteView = testPage.getByRole("button", { name: `Delete ${VIEW_NAME}` });
    await deleteView.click();
    const confirmation = testPage.getByTestId("saved-task-view-delete-confirmation");
    const deleteResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await confirmation.getByRole("button", { name: `Delete ${VIEW_NAME}` }).click();
    await deleteResponse;

    const fallbackView = testPage
      .locator('button[aria-haspopup="dialog"]')
      .filter({ hasText: "Assigned to me" });
    await expect(fallbackView).toBeVisible();
    await testPage.goto("/jira");
    await expect(
      testPage.locator('button[aria-haspopup="dialog"]').filter({ hasText: "Assigned to me" }),
    ).toBeVisible();
    const settings = await apiClient.getUserSettings();
    expect(settings.settings.jira_default_view_id).toBe("");
    expect(settings.settings.jira_saved_views).toEqual([]);
  });
});
