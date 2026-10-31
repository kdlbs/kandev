/**
 * Shared scenario setup for the plugin-attribution flows.
 *
 * The local release fixture is a real package whose manifest declares
 * `author: kandev`, while its curated pointer lives under `acme/`. The index
 * must therefore show the curated owner and the installed detail must show the
 * unchanged declaration. The fixture index is rebuilt with the real builder
 * before its source is registered so the browser consumes builder output, not
 * a hand-authored corrected response.
 */
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { PLUGIN_ID, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { PluginMarketplaceReleaseFixture } from "../../helpers/plugin-marketplace-release";

export const ATTRIBUTION_OWNER = "acme";
export const ATTRIBUTION_REPOSITORY = `${ATTRIBUTION_OWNER}/kandev-plugin-e2e`;
export const DECLARED_AUTHOR = "kandev";

export type AttributionScenario = {
  dispose: () => Promise<void>;
};

async function removeSource(apiClient: ApiClient, sourceId: string): Promise<void> {
  if (!sourceId) return;
  await apiClient
    .rawRequest("DELETE", `/api/plugins/marketplace/sources/${sourceId}`)
    .catch(() => undefined);
}

/** Start the fixture, rebuild its index, then register the corrected source. */
export async function startAttributionScenario(apiClient: ApiClient): Promise<AttributionScenario> {
  await uninstallFixturePlugin(apiClient);
  const release = await PluginMarketplaceReleaseFixture.start();
  let sourceId = "";
  try {
    await release.rebuildIndex();
    const response = await apiClient.rawRequest("POST", "/api/plugins/marketplace/sources", {
      name: "Attribution fixture",
      url: release.indexUrl,
    });
    expect(response.ok).toBe(true);
    sourceId = ((await response.json()) as { id: string }).id;
  } catch (error) {
    await removeSource(apiClient, sourceId);
    await release.close();
    throw error;
  }

  return {
    dispose: async () => {
      await removeSource(apiClient, sourceId);
      await uninstallFixturePlugin(apiClient);
      await release.close();
    },
  };
}

/** Open Settings > Plugins > Browse with the registered fixture source. */
export async function openBrowseTab(page: Page, touch = false): Promise<void> {
  await page.goto("/settings/plugins");
  const browseTab = page.getByTestId("plugins-tab-browse");
  await (touch ? browseTab.tap() : browseTab.click());
  await expect(page.getByTestId("marketplace-search")).toBeVisible();
}

export function browseEntry(page: Page): Locator {
  return page.getByTestId(`marketplace-entry-${PLUGIN_ID}`);
}

/** Browse shows the curated owner and the canonical repository, not the declaration. */
export async function expectCuratedBrowseAttribution(page: Page): Promise<void> {
  const card = browseEntry(page);
  await expect(card).toBeVisible({ timeout: 30_000 });
  await expect(card.getByText(`by ${ATTRIBUTION_OWNER}`, { exact: true })).toBeVisible();
  await expect(card.getByText(`by ${DECLARED_AUTHOR}`, { exact: true })).toHaveCount(0);
  await expect(card.getByTestId("plugin-repo-link")).toHaveAttribute(
    "href",
    `https://github.com/${ATTRIBUTION_REPOSITORY}`,
  );
}

/** The backend catalog carries the same curated attribution the card renders. */
export async function expectCatalogApiAttribution(apiClient: ApiClient): Promise<void> {
  const response = await apiClient.rawRequest("GET", "/api/plugins/marketplace");
  expect(response.ok).toBe(true);
  const catalog = (await response.json()) as {
    plugins?: Array<{ id: string; author: string; repo_url: string }>;
  };
  const entry = catalog.plugins?.find((plugin) => plugin.id === PLUGIN_ID);
  expect(entry?.author).toBe(ATTRIBUTION_OWNER);
  expect(entry?.repo_url).toBe(`https://github.com/${ATTRIBUTION_REPOSITORY}`);
}

/** The installed record keeps the manifest declaration rather than a catalog merge. */
export async function expectInstalledApiAttribution(apiClient: ApiClient): Promise<void> {
  const response = await apiClient.rawRequest("GET", "/api/plugins");
  expect(response.ok).toBe(true);
  const body = (await response.json()) as {
    plugins?: Array<{ id: string; author: string }>;
  };
  const installed = body.plugins?.find((plugin) => plugin.id === PLUGIN_ID);
  expect(installed?.author).toBe(DECLARED_AUTHOR);
}

/** Install the fixture through the visible Browse action. */
export async function installFromBrowse(page: Page, touch = false): Promise<void> {
  const installButton = page.getByTestId(`marketplace-install-${PLUGIN_ID}`);
  await expect(installButton).toBeVisible({ timeout: 30_000 });
  const installed = waitForHttp(page, "POST", /^\/api\/plugins\/install$/, {
    predicate: (response) => response.ok(),
  });
  await (touch ? installButton.tap() : installButton.click());
  await installed;
  await expect(page.getByTestId(`marketplace-installed-${PLUGIN_ID}`)).toBeVisible();
}

/** Open the installed plugin's detail page through the visible controls. */
export async function openInstalledDetail(page: Page, touch = false): Promise<void> {
  const installedTab = page.getByTestId("plugins-tab-installed");
  await (touch ? installedTab.tap() : installedTab.click());
  const row = page.getByTestId(`plugin-row-${PLUGIN_ID}`);
  await expect(row).toBeVisible({ timeout: 30_000 });
  if (touch) {
    await row.tap({ position: { x: 12, y: 12 } });
  } else {
    await page.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
  }
  await expect(page).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
  await expect(page.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
}

/** Installed details identify the manifest author as a declaration and keep raw data. */
export async function expectDeclaredAuthor(page: Page, touch = false): Promise<void> {
  const manifest = page.getByTestId("plugin-manifest-card");
  await expect(manifest).toBeVisible();
  const authorRow = manifest.getByText("Declared author", { exact: true }).locator("xpath=..");
  await expect(authorRow).toContainText(DECLARED_AUTHOR);
  const rawToggle = manifest.getByRole("button", { name: "View raw" });
  await (touch ? rawToggle.tap() : rawToggle.click());
  await expect(manifest.getByTestId("plugin-manifest-raw")).toContainText(
    `"author": "${DECLARED_AUTHOR}"`,
  );
}
