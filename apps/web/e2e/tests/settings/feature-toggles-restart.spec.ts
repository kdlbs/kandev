import path from "node:path";

import type { Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";

/**
 * Regression guard for the Feature Toggles restart action.
 *
 * The button was lost in #1389: the SPA route passed a hardcoded
 * `restartCapability={null}`, so the page always rendered the manual
 * "restart from your terminal" fallback even on installs whose backend
 * reports a restart-capable supervisor.
 *
 * Both endpoints are stubbed rather than driven for real: this asserts the UI
 * contract (capability + a pending flag => the action is offered), and never
 * clicks Restart, which would bounce the worker's backend.
 */

const CAPABILITY_URL = "**/api/v1/system/restart-capability";
const RUNTIME_FLAGS_URL = "**/api/v1/runtime-flags";

const SHOT_DIR = path.join(__dirname, "..", "..", ".restart-screenshots");

const PENDING_RESTART_FLAG = {
  key: "features.office",
  kind: "feature",
  label: "Office mode",
  description: "Enables autonomous agent office workflows and related settings.",
  stability: "experimental",
  risk_level: "medium",
  risk_description: "Office mode is still evolving.",
  effective_value: false,
  default_value: false,
  override_value: true,
  source: "override",
  env_var: "KANDEV_FEATURES_OFFICE",
  env_locked: false,
  restart_required: true,
  requires_restart_to_apply: true,
  mutable: true,
};

type Capability = { supported: boolean; mode: string; adapter?: string; reason?: string };

async function stubFeatureToggles(page: Page, capability: Capability) {
  await page.route(CAPABILITY_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(capability),
    }),
  );

  // Only intercept the list GET; PATCH /runtime-flags/:key must fall through.
  await page.route(RUNTIME_FLAGS_URL, async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ flags: [PENDING_RESTART_FLAG] }),
    });
  });
}

async function gotoFeatureToggles(page: Page) {
  await page.goto("/settings/system/feature-toggles");
  await expect(page.getByTestId("feature-toggles-settings")).toBeVisible({ timeout: 15_000 });
}

test.describe("Feature Toggles restart action", () => {
  test("offers an in-app restart when the backend reports a supervisor", async ({ testPage }) => {
    await stubFeatureToggles(testPage, {
      supported: true,
      mode: "supervisor",
      adapter: "supervisor",
    });
    await gotoFeatureToggles(testPage);

    const notice = testPage.getByText("Restart required");
    await expect(notice).toBeVisible({ timeout: 10_000 });

    // The regression: this button never rendered, whatever the backend said.
    await expect(testPage.getByRole("button", { name: "Restart", exact: true })).toBeVisible();
    await expect(testPage.getByText(/terminal or service manager/)).toHaveCount(0);

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
    await gotoFeatureToggles(testPage);

    await expect(testPage.getByText("Restart required")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(/terminal or service manager/)).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Restart", exact: true })).toHaveCount(0);

    await testPage.screenshot({
      path: path.join(SHOT_DIR, "restart-unsupported.png"),
      fullPage: false,
    });
  });

  test("fails closed to manual guidance when the capability request errors", async ({
    testPage,
  }) => {
    await testPage.route(CAPABILITY_URL, (route) => route.fulfill({ status: 500, body: "{}" }));
    await testPage.route(RUNTIME_FLAGS_URL, async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ flags: [PENDING_RESTART_FLAG] }),
      });
    });
    await gotoFeatureToggles(testPage);

    await expect(testPage.getByText("Restart required")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByRole("button", { name: "Restart", exact: true })).toHaveCount(0);
  });
});
