import { test, expect } from "../fixtures/test-base";
import { waitForHttp } from "../helpers/causal-waits";

const BOOTSTRAP_URL = "**/api/v1/system/improve-kandev/bootstrap";
const BOOTSTRAP_STATUS_URL = "**/api/v1/system/improve-kandev/bootstrap/*";
const HEALTH_URL = "**/api/v1/system/health";
const BOOTSTRAP_PATH = /^\/api\/v1\/system\/improve-kandev\/bootstrap$/;
const BOOTSTRAP_STATUS_PATH = /^\/api\/v1\/system\/improve-kandev\/bootstrap\/[^/]+$/;

test.describe("Improve Kandev on mobile", () => {
  test("dismisses the intro and reaches the issue-only task option without overflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({
      width: process.env.CAPTURE_PR_ASSETS ? 390 : 700,
      height: 900,
    });
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    await testPage.route(HEALTH_URL, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ healthy: true, issues: [] }),
      }),
    );
    await testPage.route(BOOTSTRAP_URL, (route) =>
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "mobile-job-1" }),
      }),
    );
    let firstStatusRead = true;
    await testPage.route(BOOTSTRAP_STATUS_URL, (route) => {
      if (firstStatusRead) {
        firstStatusRead = false;
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Temporary bootstrap failure" }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "Cache-Control": "no-store" },
        body: JSON.stringify({
          workspace_id: seedData.workspaceId,
          repository_id: seedData.repositoryId,
          workflow_id: seedData.workflowId,
          issue_workflow_id: seedData.workflowId,
          branch: "main",
          bundle_dir: "/tmp/kandev-improve-mobile-e2e",
          bundle_file: "/tmp/kandev-improve-mobile-e2e/diagnostic-bundle.zip",
          github_login: "octocat",
          has_write_access: false,
          fork_status: "blocked_managed",
          fork_reason_code: "managed_unavailable",
        }),
      });
    });

    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    const improveButton = testPage.getByTestId("mobile-improve-kandev-button");
    await expect(improveButton).toBeVisible();
    await improveButton.tap();

    const preference = testPage.getByTestId("improve-kandev-skip-intro");
    await expect(preference).toBeVisible();
    const preferenceBox = await preference.boundingBox();
    expect(preferenceBox?.height).toBeGreaterThanOrEqual(44);
    await preference.tap();
    const failedStatusRead = waitForHttp(testPage, "GET", BOOTSTRAP_STATUS_PATH, {
      predicate: (response) => response.status() === 500,
    });
    await testPage.getByTestId("improve-kandev-proceed").tap();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    const title = "Mobile issue fallback";
    const description = "Describe the problem";
    await createDialog.getByTestId("task-title-input").fill(title);
    await createDialog.getByTestId("task-description-input").fill(description);
    await failedStatusRead;
    const retry = createDialog.getByTestId("improve-kandev-retry");
    await expect(retry).toBeVisible({ timeout: 10_000 });
    expect((await retry.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expect(
      createDialog.getByTestId("task-create-form-body").getByText("Temporary bootstrap failure"),
    ).toBeVisible();
    await expect(
      createDialog
        .getByTestId("toast-message")
        .filter({ hasText: "Could not prepare Improve Kandev" }),
    ).toBeVisible();
    await prCapture.screenshot("phone-bootstrap-retry", {
      caption: "Phone Improve Kandev form with its draft preserved and touch-sized inline Retry",
    });
    const retryAdmission = waitForHttp(testPage, "POST", BOOTSTRAP_PATH, {
      predicate: (response) => response.status() === 202,
    });
    const readyStatusRead = waitForHttp(testPage, "GET", BOOTSTRAP_STATUS_PATH, {
      predicate: (response) => response.status() === 200,
    });
    await retry.tap();
    await Promise.all([retryAdmission, readyStatusRead]);
    await expect(
      createDialog.getByText(
        "Managed GitHub access could not prepare a verified fork for this task. Check the workspace GitHub connection or open an issue instead.",
      ),
    ).toBeVisible({ timeout: 10_000 });
    await expect(createDialog.getByTestId("task-title-input")).toHaveValue(title);
    await expect(createDialog.getByTestId("task-description-input")).toContainText(description);
    await expect(createDialog.getByTestId("submit-start-agent")).toBeDisabled();
    await createDialog.getByRole("tab", { name: "Open issue" }).tap();
    await expect(
      createDialog.getByText(/only create a GitHub issue.*will not implement/i),
    ).toBeVisible();
    await expect(createDialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 10_000 });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
