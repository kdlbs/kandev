import { test, expect } from "../../fixtures/test-base";

const inotifyErrorResponse = {
  healthy: false,
  issues: [
    {
      id: "os_inotify_instances_high",
      category: "system_resources",
      title: "Inotify instances limit nearly exhausted",
      message:
        "123/128 instances in use (96%). Exhaustion causes new terminals, dev servers, and agent CLIs to fail or hang. To increase: sudo sysctl -w fs.inotify.max_user_instances=1024",
      severity: "error",
      fix_url: "/settings/system/status",
      fix_label: "View system status",
    },
  ],
  checks: [
    { name: "OS resource limits", category: "system_resources", passing: false },
    { name: "GitHub integration", category: "github", passing: true },
    { name: "AI agent availability", category: "agents", passing: true },
  ],
};

test.describe("System Status page", () => {
  test("renders health card, version summary, and the page title", async ({ testPage }) => {
    await testPage.goto("/settings/system/status");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Status");
    await expect(testPage.getByTestId("system-health-card")).toBeVisible();
    await expect(testPage.getByTestId("system-version-summary-card")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-card")).toBeVisible();
    await expect(testPage.getByTestId("system-ui-state-card")).toBeVisible();
  });

  test("inner System > Status breadcrumb is no longer rendered", async ({ testPage }) => {
    // The outer Home > Settings breadcrumb from PageTopbar is enough; the
    // duplicated inner one inside SystemPageShell was removed.
    await testPage.goto("/settings/system/status");
    await expect(testPage.getByTestId("system-page-title")).toBeVisible();
    await expect(testPage.getByTestId("system-breadcrumb")).toHaveCount(0);
  });

  test("health card info popover lists the system checks that ran", async ({ testPage }) => {
    await testPage.goto("/settings/system/status");

    const trigger = testPage.getByTestId("system-health-checks-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });

    await trigger.click();
    const popover = testPage.getByTestId("system-health-checks-popover");
    await expect(popover).toBeVisible();
    // The default backend wiring registers GitHub, agents, and OS resource limits checks.
    await expect(testPage.getByTestId("system-health-check-github")).toBeVisible();
    await expect(testPage.getByTestId("system-health-check-agents")).toBeVisible();
    await expect(testPage.getByTestId("system-health-check-system_resources")).toBeVisible();
  });

  test("health card shows inotify error when returned by the API", async ({
    testPage,
    backend,
  }) => {
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(inotifyErrorResponse),
      }),
    );

    await testPage.goto("/settings/system/status");

    await expect(
      testPage.getByTestId("system-health-issue-os_inotify_instances_high"),
    ).toBeVisible();
    await expect(
      testPage.getByTestId("system-health-issue-os_inotify_instances_high"),
    ).toContainText("Inotify instances limit nearly exhausted");
    await expect(
      testPage.getByTestId("system-health-issue-os_inotify_instances_high"),
    ).toContainText("sysctl");
  });

  test("health checks popover includes system_resources check", async ({ testPage, backend }) => {
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(inotifyErrorResponse),
      }),
    );

    await testPage.goto("/settings/system/status");

    const trigger = testPage.getByTestId("system-health-checks-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();

    const popover = testPage.getByTestId("system-health-checks-popover");
    await expect(popover).toBeVisible();
    await expect(testPage.getByTestId("system-health-check-system_resources")).toBeVisible();
  });

  test("UI state Reset button is wired up and triggers a reload", async ({ testPage }) => {
    await testPage.goto("/settings/system/status");

    const resetButton = testPage.getByTestId("system-ui-state-reset");
    await expect(resetButton).toBeVisible();

    // Sentinel value: if Reset wipes localStorage as designed, this key will
    // not survive the click + reload that follows.
    await testPage.evaluate(() => window.localStorage.setItem("e2e-ui-state-sentinel", "1"));

    const navigation = testPage.waitForLoadState("load");
    await resetButton.click();
    await navigation;

    const stored = await testPage.evaluate(() =>
      window.localStorage.getItem("e2e-ui-state-sentinel"),
    );
    expect(stored).toBeNull();
  });
});
