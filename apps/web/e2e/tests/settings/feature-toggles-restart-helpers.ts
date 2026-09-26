import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";

export const PENDING_RESTART_FLAG = {
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

const CAPABILITY_URL = "**/api/v1/system/restart-capability";
const RUNTIME_FLAGS_URL = "**/api/v1/runtime-flags";

export async function stubFeatureToggles(page: Page, capability: Capability) {
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

export async function stubCapabilityFailure(page: Page) {
  await page.route(CAPABILITY_URL, (route) => route.fulfill({ status: 500, body: "{}" }));
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

export async function gotoFeatureToggles(page: Page): Promise<Locator> {
  await page.goto("/settings/system/feature-toggles");
  const settings = page.getByTestId("feature-toggles-settings");
  await expect(settings).toBeVisible({ timeout: 15_000 });
  return settings;
}
