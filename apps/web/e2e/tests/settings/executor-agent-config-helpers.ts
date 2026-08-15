import { expect, type Locator, type Page } from "@playwright/test";

export const PORTABLE_CONFIG_BUNDLE_ID = "mock.settings";

export function portableConfigSection(page: Page): Locator {
  return page.getByTestId("portable-config-bundles");
}

export async function selectPortableConfigBundle(
  page: Page,
  bundleId = PORTABLE_CONFIG_BUNDLE_ID,
): Promise<void> {
  const row = page.getByTestId(`portable-config-bundle-${bundleId}`);
  await expect(row).toBeVisible();
  await row.getByRole("checkbox").check();
}

export function selectedBundleIds(config: Record<string, string> | undefined): string[] {
  const raw = config?.agent_config_bundles;
  if (!raw) return [];
  return JSON.parse(raw) as string[];
}

export async function expectTouchTarget(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

export async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflow).toBe(false);
}
