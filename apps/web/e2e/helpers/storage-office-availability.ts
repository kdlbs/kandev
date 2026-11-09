import { expect, type Page } from "@playwright/test";
import type { BootPayload } from "@/src/boot-payload";
import { dwell } from "./causal-waits";

export async function setOfficeAvailability(page: Page, enabled: boolean) {
  await page.addInitScript((office) => {
    let boot: BootPayload | undefined;
    Object.defineProperty(window, "__KANDEV_BOOT_PAYLOAD__", {
      configurable: true,
      get: () => boot,
      set: (payload: BootPayload) => {
        if (payload.initialState?.features) payload.initialState.features.office = office;
        boot = payload;
      },
    });
  }, enabled);
}

// @covers AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2/.6/.7/.8
export async function checkOfficeAvailability(page: Page, enabled: boolean, phone: boolean) {
  await setOfficeAvailability(page, enabled);
  let retentionRequests = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/system/retention") retentionRequests++;
  });
  const storage = "/settings/system/storage";
  const paths = enabled
    ? [`${storage}?tab=office-retention`]
    : [
        storage,
        `${storage}?tab=office-retention`,
        `${storage}#setting-system-retention`,
        `${storage}?tab=office-retention#setting-system-retention`,
        "/settings/system/data-storage?preserve=1#setting-system-retention",
      ];
  for (const path of paths) {
    await page.goto(path);
    const tab = page.getByRole("tab", { name: enabled ? "Office retention" : "Host", exact: true });
    await expect(tab).toHaveAttribute("aria-selected", "true");
    if (enabled) {
      await expect(page.getByTestId("retention-policy-card")).toBeVisible();
      await expect.poll(() => retentionRequests).toBeGreaterThan(0);
    } else {
      await expect(page.getByRole("tab", { name: "Office retention", exact: true })).toHaveCount(0);
      await expect(page.getByTestId("settings-storage-office-retention")).toHaveCount(0);
      await expect(page.getByTestId("storage-settings-page")).toBeVisible();
      if (phone) await tab.tap();
      else await tab.click();
    }
    if (phone) {
      expect((await tab.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    }
  }
  if (!enabled) {
    await dwell(
      page,
      300,
      "negative-assertion",
      "Disabled retention must not start a request after Storage settles",
    );
    expect(retentionRequests).toBe(0);
  }
  await checkRetentionSearch(page, enabled, phone);
}

async function checkRetentionSearch(page: Page, enabled: boolean, phone: boolean) {
  const storage = "/settings/system/storage";
  await page.goto(phone ? "/settings" : storage);
  const navigation = page.getByTestId(phone ? "settings-index" : "app-sidebar-settings-mode");
  await navigation.getByRole("searchbox", { name: "Search settings" }).fill("Office retention");
  const result = navigation.locator(
    'a[href="/settings/system/storage?tab=office-retention#setting-system-retention"]',
  );
  if (enabled) {
    await expect(result).toBeVisible();
    if (phone) await result.tap();
    else await result.click();
    await expect(page.getByTestId("retention-policy-card")).toBeVisible();
  } else {
    await expect(navigation.getByText("No matching settings", { exact: true })).toBeVisible();
    await expect(result).toHaveCount(0);
    await page.goto(storage);
    await expect(page.getByTestId("storage-settings-page")).toBeVisible();
  }
}
