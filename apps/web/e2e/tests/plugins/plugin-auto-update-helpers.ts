import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
import {
  installFixturePlugin,
  uninstallFixturePlugin,
  PLUGIN_ID,
} from "../../helpers/plugin-fixture";

const SETTINGS = "/api/plugins/settings";

type Receipt = { release: () => void; done: Promise<void> };

export async function exerciseAutoUpdateIntent(
  page: Page,
  api: ApiClient,
  touch: boolean,
  success: boolean,
) {
  const read = async () => {
    const response = await api.rawRequest("GET", SETTINGS);
    expect(response.ok).toBe(true);
    return (await response.json()) as { auto_update_default: boolean };
  };
  const write = async (value: boolean) => {
    const response = await api.rawRequest("PUT", SETTINGS, { auto_update_default: value });
    expect(response.ok).toBe(true);
  };
  const baseline = await read();
  const receipts: Receipt[] = [];
  let admittedWrites = 0;
  const routePattern = "**/api/plugins/settings";
  try {
    await installFixturePlugin(page);
    const plugins = await api.rawRequest("GET", "/api/plugins");
    const inventory = (await plugins.json()) as { plugins: Array<{ id: string }> };
    expect(inventory.plugins.map((plugin) => plugin.id)).toEqual([PLUGIN_ID]);
    const catalog = await api.rawRequest("GET", "/api/plugins/marketplace");
    const entries = (await catalog.json()) as {
      plugins: Array<{ id: string }>;
    };
    expect(entries.plugins.some((plugin) => plugin.id === PLUGIN_ID)).toBe(false);
    await write(false);
    await page.reload();
    const global = page.getByTestId("plugins-auto-update-default");
    const inherited = page.getByTestId(`plugin-auto-update-${PLUGIN_ID}`);
    await expect(global).toHaveCount(1);
    await expect(inherited).toHaveCount(1);
    await expect(global).toBeEnabled();
    await expect(global).toHaveAttribute("aria-checked", "false");
    await expect(inherited).toHaveAttribute("aria-checked", "false");

    await page.route(routePattern, async (route) => {
      if (route.request().method() !== "PUT") return route.continue();
      admittedWrites += 1;
      let release!: () => void;
      let done!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      const settled = new Promise<void>((resolve) => {
        done = resolve;
      });
      // Fetch commits to the real disposable backend; only the receipt is held.
      const response = success ? await route.fetch() : undefined;
      if (response) expect(response.ok()).toBe(true);
      receipts.push({ release, done: settled });
      try {
        await held;
        if (response) await route.fulfill({ response });
        else await route.fulfill({ status: 503, json: { error: "preference write failed" } });
      } finally {
        done();
      }
    });
    const toggle = async () => {
      if (touch) await global.tap();
      else await global.click();
    };
    await toggle();
    await expect.poll(() => receipts.length).toBe(1);
    if (success) expect((await read()).auto_update_default).toBe(true);
    await expect(inherited).toHaveAttribute("aria-checked", "true");
    await toggle();
    await expect(inherited).toHaveAttribute("aria-checked", "false");
    await dwell(
      page,
      150,
      "negative-assertion",
      "A later choice must not enter HTTP before the held receipt settles",
    );
    expect(admittedWrites).toBe(1);
    receipts[0].release();
    await receipts[0].done;
    await expect.poll(() => receipts.length).toBe(2);
    await expect(global).toHaveAttribute("aria-checked", "false");
    await expect(inherited).toHaveAttribute("aria-checked", "false");
    receipts[1].release();
    await receipts[1].done;
    if (!success) {
      await expect(page.getByText("preference write failed", { exact: true })).toHaveCount(2);
    }
    await expect(global).toHaveAttribute("aria-checked", "false");
    await expect(inherited).toHaveAttribute("aria-checked", "false");
    expect((await read()).auto_update_default).toBe(false);
    await page.unroute(routePattern);
    await page.reload();
    await expect(global).toBeEnabled();
    await expect(global).toHaveAttribute("aria-checked", "false");
    await expect(inherited).toHaveAttribute("aria-checked", "false");
    if (touch) await page.screenshot({ path: test.info().outputPath("plugin-auto-update.png") });
  } finally {
    // Release owned responses before disposing the page and restoring settings.
    for (const receipt of receipts) receipt.release();
    if (!page.isClosed()) await page.goto("about:blank").catch(() => undefined);
    await Promise.all(receipts.map((receipt) => receipt.done));
    if (!page.isClosed()) await page.unroute(routePattern);
    await write(baseline.auto_update_default);
    expect((await read()).auto_update_default).toBe(baseline.auto_update_default);
    await uninstallFixturePlugin(api);
  }
}
