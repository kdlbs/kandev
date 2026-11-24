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

  test("UI state Reset clears browser storage and reloads Status", async ({ testPage }) => {
    await testPage.goto("/settings/system/status");

    const resetButton = testPage.getByTestId("system-ui-state-reset");
    await expect(resetButton).toBeVisible();

    const storageKey = "e2e-ui-state-sentinel";
    const previousDocument = await testPage.evaluate((key) => {
      localStorage.setItem(key, "1");
      sessionStorage.setItem(key, "1");
      return performance.timeOrigin;
    }, storageKey);

    const statusUrl = testPage.url();
    await Promise.all([
      testPage.waitForEvent(
        "framenavigated",
        (frame) => frame === testPage.mainFrame() && frame.url() === statusUrl,
      ),
      resetButton.click(),
    ]);
    await expect(resetButton).toBeVisible();

    const reloaded = await testPage.evaluate(
      (key) => ({
        document: performance.timeOrigin,
        local: localStorage.getItem(key),
        session: sessionStorage.getItem(key),
      }),
      storageKey,
    );
    expect(reloaded.document).not.toBe(previousDocument);
    expect(reloaded).toMatchObject({ local: null, session: null });
  });
});
