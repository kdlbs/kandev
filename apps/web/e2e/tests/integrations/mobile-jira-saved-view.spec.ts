import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile Jira saved views", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockJiraReset();
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");
  });

  test("keeps custom-view confirmation inline, touch-sized, and contained", async ({
    testPage,
    apiClient,
  }) => {
    const seedResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      jira_saved_views: [
        {
          id: "jira-mobile-long-view",
          name: "Long mobile sprint bugs that need review",
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

    await testPage.getByRole("button", { name: "Assigned to me" }).tap();
    const deleteAction = testPage.getByRole("button", {
      name: "Delete Long mobile sprint bugs that need review",
    });
    const deleteBox = await deleteAction.boundingBox();
    expect(deleteBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(deleteBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    await deleteAction.tap();

    const confirmation = testPage.getByTestId("saved-task-view-delete-confirmation");
    await expect(confirmation).toHaveAccessibleName(
      "Delete Long mobile sprint bugs that need review?",
    );
    await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
    for (const action of await confirmation.getByRole("button").all()) {
      const box = await action.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Jira saved-view confirmation");
    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(deleteAction).toBeFocused();

    await deleteAction.tap();
    const deleteResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await testPage
      .getByTestId("saved-task-view-delete-confirmation")
      .getByRole("button", { name: "Delete Long mobile sprint bugs that need review" })
      .tap();
    await deleteResponse;
    await expect(deleteAction).toHaveCount(0);
  });
});
