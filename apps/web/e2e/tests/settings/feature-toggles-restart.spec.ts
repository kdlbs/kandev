import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import {
  gotoFeatureToggles,
  stubCapabilityFailure,
  stubFeatureToggles,
} from "./feature-toggles-restart-helpers";

const SHOT_DIR = path.join(__dirname, "..", "..", ".restart-screenshots");

/**
 * Regression guard for the Feature Toggles restart action.
 *
 * Both endpoints are stubbed rather than driven for real: this asserts the UI
 * contract (capability + a pending flag => the action is offered), and never
 * clicks Restart, which would bounce the worker's backend.
 */
test.describe("Feature Toggles restart action", () => {
  test("offers an in-app restart when the backend reports a supervisor", async ({ testPage }) => {
    await stubFeatureToggles(testPage, {
      supported: true,
      mode: "supervisor",
      adapter: "supervisor",
    });
    const settings = await gotoFeatureToggles(testPage);

    const notice = settings.getByText("Restart required");
    await expect(notice).toBeVisible({ timeout: 10_000 });

    // The regression: this button never rendered, whatever the backend said.
    await expect(settings.getByRole("button", { name: "Restart", exact: true })).toBeVisible();
    await expect(settings.getByText(/terminal or service manager/)).toHaveCount(0);

    await testPage.screenshot({
      path: path.join(SHOT_DIR, "restart-supported.png"),
      fullPage: false,
    });
  });

  test("falls back to manual guidance when restart is unsupported", async ({ testPage }) => {
    await stubFeatureToggles(testPage, {
      supported: false,
      mode: "manual",
      reason: "Automatic restart is not available for this launch mode.",
    });
    const settings = await gotoFeatureToggles(testPage);

    await expect(settings.getByText("Restart required")).toBeVisible({ timeout: 10_000 });
    await expect(settings.getByText(/terminal or service manager/)).toBeVisible();
    await expect(settings.getByRole("button", { name: "Restart", exact: true })).toHaveCount(0);

    await testPage.screenshot({
      path: path.join(SHOT_DIR, "restart-unsupported.png"),
      fullPage: false,
    });
  });

  test("fails closed to manual guidance when the capability request errors", async ({
    testPage,
  }) => {
    await stubCapabilityFailure(testPage);
    const settings = await gotoFeatureToggles(testPage);

    await expect(settings.getByText("Restart required")).toBeVisible({ timeout: 10_000 });
    await expect(settings.getByRole("button", { name: "Restart", exact: true })).toHaveCount(0);
  });
});
