import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForHttp } from "../../helpers/causal-waits";

const VIEW_ID = "jira-mobile-default-open-tickets";
const VIEW_NAME = "My open tickets";
const CUSTOM_JQL = 'project = CLIP AND status = "In Development" ORDER BY updated DESC';

test.describe("Mobile Jira default view", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockJiraReset();
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");
    await apiClient.mockJiraSetProjects([{ id: "1", key: "CLIP", name: "Clip" }]);
    await apiClient.mockJiraSetProjectStatuses("CLIP", [
      { id: "10001", name: "In Development", statusCategory: "indeterminate" },
    ]);
    await apiClient.mockJiraSetSearchHits([
      {
        key: "CLIP-1",
        summary: "Development ticket",
        projectKey: "CLIP",
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
            projectKeys: ["CLIP"],
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

  test("sets and clears the default through touch controls and restores both visits", async ({
    apiClient,
    testPage,
  }) => {
    await testPage.goto("/jira");
    const assignedView = testPage
      .locator('button[aria-haspopup="dialog"]')
      .filter({ hasText: "Assigned to me" });
    await assignedView.tap();
    const setDefault = testPage.getByRole("button", {
      name: `Set ${VIEW_NAME} as default view`,
    });
    const setBox = await setDefault.boundingBox();
    expect(setBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(setBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    const saveDefault = waitForHttp(testPage, "PATCH", /^\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.ok(),
    });
    await setDefault.tap();
    await saveDefault;
    await expect(assignedView).toBeVisible({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Jira default-view picker");
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

    await restoredView.tap();
    const clearDefault = testPage.getByRole("button", {
      name: `Clear ${VIEW_NAME} as default view`,
    });
    const clearBox = await clearDefault.boundingBox();
    expect(clearBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(clearBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    const clearResponse = waitForHttp(testPage, "PATCH", /^\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.ok(),
    });
    await clearDefault.tap();
    await clearResponse;
    await expect(restoredView).toBeVisible({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Jira default-view picker");
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.jira_default_view_id)
      .toBe("");

    await testPage.goto("/jira");
    await expect(
      testPage.locator('button[aria-haspopup="dialog"]').filter({ hasText: "Assigned to me" }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
