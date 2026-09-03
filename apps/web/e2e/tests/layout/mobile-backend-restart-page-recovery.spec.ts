import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile backend restart page recovery", () => {
  test("keeps the recovery action reachable without horizontal overflow", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/stats");
    await expect(testPage.getByTestId("app-shell")).toBeVisible();
    await testPage.waitForLoadState("networkidle");

    const systemInfoRecovered = waitForHttp(testPage, "GET", /^\/api\/v1\/system\/info$/, {
      predicate: (response) => response.ok(),
    });
    await backend.restart();
    await systemInfoRecovered;

    const alert = testPage.getByTestId("backend-reload-required-alert");
    await expect(alert).toBeVisible();
    const action = alert.getByRole("button", { name: "Reload page" });
    const actionBox = await action.boundingBox();
    expect(actionBox, "backend restart recovery action has no rendered hitbox").not.toBeNull();
    expect(actionBox?.height ?? 0).toBeGreaterThanOrEqual(44);

    const main = testPage.locator("main").first();
    const alertBox = await alert.boundingBox();
    const mainBox = await main.boundingBox();
    expect(alertBox).not.toBeNull();
    expect(mainBox).not.toBeNull();
    if (alertBox && mainBox) {
      expect(alertBox.y + alertBox.height).toBeLessThanOrEqual(mainBox.y + 1);
    }

    await assertNoDocumentHorizontalOverflow(testPage, "mobile backend restart recovery");
    await prCapture.screenshot("mobile-backend-restart-page-recovery", {
      caption: "Persistent mobile backend restart recovery alert",
    });

    await action.click();
    await expect(testPage.getByTestId("app-shell")).toBeVisible();
    await expect(testPage.getByTestId("backend-reload-required-alert")).toHaveCount(0, {
      timeout: 30_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile backend restart recovery reload");
  });
});
