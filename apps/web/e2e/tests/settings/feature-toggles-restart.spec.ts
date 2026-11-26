import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import {
  ACP_V2_CARD,
  gotoFeatureToggles,
  saveAcpV2RestartOverride,
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
  test("saves an ACP v2 opt-in for the next restart", async ({ testPage }) => {
    await saveAcpV2RestartOverride(testPage, false);
  });

  test("locks the ACP v2 toggle when the environment controls it", async ({
    backend,
    testPage,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_ACP_V2: "true" });
    try {
      const settings = await gotoFeatureToggles(testPage);
      const card = settings.getByTestId(ACP_V2_CARD);
      const toggle = card.getByRole("switch", { name: "Toggle ACP v2 preview" });
      await expect(toggle).toBeChecked();
      await expect(toggle).toBeDisabled();
    } finally {
      await release();
    }
  });

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
