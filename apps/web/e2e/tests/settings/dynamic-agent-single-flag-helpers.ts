import { expect, type Locator, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import { waitForHttp } from "../../helpers/causal-waits";
import type { RuntimeFlagsResponse } from "../../../lib/types/runtime-flags";

export const DYNAMIC_AGENT_ROUTING_KEY = "features.dynamicAgentRouting";
export const RETIRED_CIRCUIT_BACKOFF_KEY = "features.dynamicCircuitBackoffEscalation";

export function useDynamicCircuitFlagEnvironment(
  backend: BackendContext,
  dynamicAgentRouting: boolean,
  staleBackoffValue: boolean,
) {
  return backend.useEnv({
    KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: String(dynamicAgentRouting),
    KANDEV_FEATURES_DYNAMIC_CIRCUIT_BACKOFF_ESCALATION: String(staleBackoffValue),
  });
}

export async function openDynamicAgentFlagCatalog(page: Page, reload = false) {
  const catalogResponse = waitForHttp(page, "GET", /\/api\/v1\/runtime-flags$/);
  if (reload) {
    await page.reload();
  } else {
    await page.goto("/settings/system/feature-toggles");
  }
  const settings = page.getByTestId("feature-toggles-settings");
  await expect(settings).toBeVisible();
  const response = await catalogResponse;
  expect(response.ok()).toBe(true);
  return { settings, catalog: (await response.json()) as RuntimeFlagsResponse };
}

export async function assertSingleDynamicAgentToggle(
  page: Page,
  settings: Locator,
  catalog: RuntimeFlagsResponse,
  expectedValue: boolean,
) {
  const dynamicCard = page.getByTestId(`feature-toggle-${DYNAMIC_AGENT_ROUTING_KEY}`);
  const retiredCard = page.getByTestId(`feature-toggle-${RETIRED_CIRCUIT_BACKOFF_KEY}`);
  await expect(settings).toContainText("Dynamic agent routing");
  await expect(dynamicCard).toHaveCount(1);
  await expect(retiredCard).toHaveCount(0);
  await expect(dynamicCard.getByRole("switch")).toHaveAttribute(
    "aria-checked",
    String(expectedValue),
  );
  await expect(dynamicCard.getByRole("switch")).toBeDisabled();

  const dynamicFlag = catalog.flags.find((flag) => flag.key === DYNAMIC_AGENT_ROUTING_KEY);
  expect(dynamicFlag).toBeDefined();
  expect(dynamicFlag?.effective_value).toBe(expectedValue);
  expect(dynamicFlag?.env_locked).toBe(true);
  expect(catalog.flags.some((flag) => flag.key === RETIRED_CIRCUIT_BACKOFF_KEY)).toBe(false);

  await page.waitForFunction(() => {
    const bootWindow = window as Window & {
      __KANDEV_BOOT_PAYLOAD__?: {
        initialState?: { features?: Record<string, unknown> };
      };
    };
    return Boolean(bootWindow.__KANDEV_BOOT_PAYLOAD__?.initialState?.features);
  });
  const bootFeatures = await page.evaluate(() => {
    const bootWindow = window as Window & {
      __KANDEV_BOOT_PAYLOAD__?: {
        initialState?: { features?: Record<string, unknown> };
      };
    };
    return bootWindow.__KANDEV_BOOT_PAYLOAD__?.initialState?.features ?? null;
  });
  expect(bootFeatures).not.toBeNull();
  expect(bootFeatures).toHaveProperty("dynamicAgentRouting", expectedValue);
  expect(bootFeatures).not.toHaveProperty("dynamicCircuitBackoffEscalation");
}
