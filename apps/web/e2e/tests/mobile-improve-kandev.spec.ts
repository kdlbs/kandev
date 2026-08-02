import { test, expect } from "../fixtures/test-base";

const BOOTSTRAP_URL = "**/api/v1/system/improve-kandev/bootstrap";
const HEALTH_URL = "**/api/v1/system/health";

test.describe("Improve Kandev on mobile", () => {
  test("dismisses the intro and reaches the issue-only task option without overflow", async ({
    testPage,
    seedData,
  }) => {
    await testPage.route(HEALTH_URL, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ healthy: true, issues: [] }),
      }),
    );
    await testPage.route(BOOTSTRAP_URL, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          repository_id: seedData.repositoryId,
          workflow_id: seedData.workflowId,
          issue_workflow_id: seedData.workflowId,
          branch: "main",
          bundle_dir: "/tmp/kandev-improve-mobile-e2e",
          bundle_file: "/tmp/kandev-improve-mobile-e2e/diagnostic-bundle.zip",
          github_login: "octocat",
          has_write_access: false,
          fork_status: "unknown",
        }),
      }),
    );

    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Open menu" }).tap();
    const improveButton = testPage.getByTestId("mobile-improve-kandev-button");
    await expect(improveButton).toBeVisible();
    await improveButton.tap();

    const preference = testPage.getByTestId("improve-kandev-skip-intro");
    await expect(preference).toBeVisible();
    const preferenceBox = await preference.boundingBox();
    expect(preferenceBox?.height).toBeGreaterThanOrEqual(44);
    await preference.tap();
    await testPage.getByTestId("improve-kandev-proceed").tap();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    await createDialog.getByRole("tab", { name: "Open issue" }).tap();
    await expect(
      createDialog.getByText(/only create a GitHub issue.*will not implement/i),
    ).toBeVisible();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
