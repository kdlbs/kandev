import { expect, test } from "../../fixtures/test-base";

test.describe("Backend restart page recovery", () => {
  test("shows one reload alert on the current non-settings route", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/stats");
    await expect(testPage.getByTestId("app-shell")).toBeVisible();
    await expect(testPage.getByTestId("backend-reload-required-alert")).toHaveCount(0);
    await testPage.waitForLoadState("networkidle");

    await backend.restart();

    const alert = testPage.getByTestId("backend-reload-required-alert");
    await expect(alert).toBeVisible({ timeout: 30_000 });
    await expect(alert).toContainText("Reload required");
    await expect(alert).toContainText("Kandev restarted");
    await expect(testPage.getByRole("button", { name: "Reload page" })).toHaveCount(1);
    await expect(testPage).toHaveURL(/\/stats$/);
    await prCapture.screenshot("backend-restart-page-recovery-desktop", {
      caption: "Persistent desktop backend restart recovery alert",
    });

    await alert.getByRole("button", { name: "Reload page" }).click();
    await expect(testPage.getByTestId("app-shell")).toBeVisible();
    await expect(testPage.getByTestId("backend-reload-required-alert")).toHaveCount(0, {
      timeout: 30_000,
    });
    await expect(testPage).toHaveURL(/\/stats$/);
  });
});
