import { test, expect } from "../../fixtures/test-base";

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
    // The default backend wiring registers GitHub + agents checks.
    await expect(testPage.getByTestId("system-health-check-github")).toBeVisible();
    await expect(testPage.getByTestId("system-health-check-agents")).toBeVisible();
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
