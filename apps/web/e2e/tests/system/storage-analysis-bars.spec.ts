import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import {
  mockStorageAnalysisBarsOverview,
  storageBarsSnapshot,
} from "../../helpers/storage-maintenance";

const DEFAULT_ORDER = [
  "database",
  "database-backups",
  "workspaces",
  "go-cache",
  "unmanaged-go-cache",
  "quarantine",
  "temporary-artifacts",
  "system-temporary",
  "managed-containers",
  "docker-image-layers",
  "docker-build-cache",
  "docker-unused-images",
  "docker-networks",
];

function resourceTriggers(page: Page) {
  return page.locator('[data-testid^="storage-resource-"][data-testid$="-trigger"]');
}

async function resourceOrder(page: Page) {
  return resourceTriggers(page).evaluateAll((elements) =>
    elements.map((element) => {
      const testId = element.getAttribute("data-testid") ?? "";
      return testId.replace(/^storage-resource-/, "").replace(/-trigger$/, "");
    }),
  );
}

async function expectResourceExpanded(page: Page, resourceId: string) {
  const resource = page.getByTestId(`storage-resource-${resourceId}`);
  await expect(resource.getByTestId(`storage-resource-${resourceId}-trigger`)).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(resource.locator('[data-slot="accordion-content"]')).toBeVisible();
}

test.describe("Storage analysis bars", () => {
  test("sorts measured rows, scales decorative bars, and preserves totals", async ({
    testPage,
    prCapture,
  }) => {
    const { setSnapshot } = await mockStorageAnalysisBarsOverview(testPage);
    await testPage.goto("/settings/system/storage");

    await expect(testPage.getByTestId("storage-analysis-bars-description")).toContainText(
      "Bars compare category sizes. Categories can overlap.",
    );
    await expect.poll(() => resourceOrder(testPage)).toEqual(DEFAULT_ORDER);

    const totalText = await testPage.getByTestId("storage-analysis-total").textContent();
    expect(totalText).toContain("Total counted");
    await expect(testPage.getByTestId("storage-resource-database-bar-fill")).toHaveAttribute(
      "style",
      "width: 100%;",
    );
    await expect(testPage.getByTestId("storage-resource-workspaces-bar-fill")).toHaveAttribute(
      "style",
      "width: 80%;",
    );

    const bars = testPage.locator('[data-testid^="storage-resource-"][data-testid$="-bar"]');
    for (const bar of await bars.all()) {
      await expect(bar).toHaveAttribute("aria-hidden", "true");
      expect(await bar.getAttribute("role")).toBeNull();
    }

    const databaseTrigger = testPage.getByTestId("storage-resource-database-trigger");
    const databaseBar = testPage.getByTestId("storage-resource-database-bar");
    const triggerBox = await databaseTrigger.boundingBox();
    const barBox = await databaseBar.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(barBox).not.toBeNull();
    expect(barBox!.x).toBeGreaterThanOrEqual(triggerBox!.x);
    expect(barBox!.x + barBox!.width).toBeLessThanOrEqual(triggerBox!.x + triggerBox!.width + 1);
    expect(barBox!.width).toBeGreaterThan(0);

    // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.6: all desktop tracks share one column.
    const rowGeometry = await Promise.all(
      ["database", "system-temporary", "workspaces", "go-cache"].map(async (id) => ({
        bar: await testPage.getByTestId(`storage-resource-${id}-bar`).boundingBox(),
        value: await testPage
          .getByTestId(`storage-resource-${id}-trigger`)
          .locator('[data-testid^="storage-analysis-source-"]')
          .boundingBox(),
      })),
    );
    for (const { bar, value } of rowGeometry) {
      expect(bar).not.toBeNull();
      expect(value).not.toBeNull();
      expect(Math.abs(bar!.x - barBox!.x)).toBeLessThanOrEqual(1);
      expect(Math.abs(bar!.width - barBox!.width)).toBeLessThanOrEqual(1);
    }

    const systemTrigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await systemTrigger.click();
    await expect(testPage.getByTestId("storage-resource-system-temporary")).toContainText(
      "Scan timed out. Showing partial usage.",
    );
    await expect(systemTrigger).toContainText("Partial");
    expect(totalText).toBe(await testPage.getByTestId("storage-analysis-total").textContent());
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await prCapture.screenshot("storage-analysis-bars", {
      caption: "Desktop storage analysis compares measured category sizes with relative bars",
      fullPage: true,
    });

    setSnapshot({
      ...storageBarsSnapshot({ workspaces: 8 * 1024 ** 3, quarantine: 0 }),
      database: { status: "unavailable", included_in_total: false },
      temporary_artifacts: { available: true },
      go_cache: { available: false },
      docker: {
        available: false,
        managed_container_count: 0,
        managed_container_bytes: 0,
        image_layer_bytes: 0,
        build_cache_bytes: 0,
        unused_image_bytes: 0,
      },
    });
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
    );
    await expect(testPage.getByTestId("storage-resource-quarantine-bar-fill")).toHaveAttribute(
      "style",
      "width: 0%;",
    );
    await expect(testPage.getByTestId("storage-resource-database-bar")).toHaveCount(0);
    await expect(testPage.getByTestId("storage-resource-temporary-artifacts-bar")).toHaveCount(0);
    const unavailableValue = await testPage
      .getByTestId("storage-resource-managed-containers-trigger")
      .locator('[data-testid="storage-analysis-source-docker"]')
      .boundingBox();
    const measuredValue = await testPage
      .getByTestId("storage-resource-workspaces-trigger")
      .locator('[data-testid="storage-analysis-source-workspaces"]')
      .boundingBox();
    expect(unavailableValue).not.toBeNull();
    expect(measuredValue).not.toBeNull();
    expect(Math.abs(unavailableValue!.x - measuredValue!.x)).toBeLessThanOrEqual(1);
  });

  test("keeps expanded rows and focus attached to IDs after refresh reordering", async ({
    testPage,
  }) => {
    const { setSnapshot, holdNextOverviewRefresh } =
      await mockStorageAnalysisBarsOverview(testPage);
    setSnapshot(storageBarsSnapshot({ goCache: 16 * 1024 ** 3 }));
    const cleanupRequests: string[] = [];
    testPage.on("request", (request) => {
      if (
        request.method() === "POST" &&
        new URL(request.url()).pathname === "/api/v1/system/storage/run"
      ) {
        cleanupRequests.push(request.url());
      }
    });
    await testPage.goto("/settings/system/storage");

    const systemTrigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    const workspaceTrigger = testPage.getByTestId("storage-resource-workspaces-trigger");
    await systemTrigger.click();
    await workspaceTrigger.click();
    await expectResourceExpanded(testPage, "system-temporary");
    await expectResourceExpanded(testPage, "workspaces");

    const goCacheTrigger = testPage.getByTestId("storage-resource-go-cache-trigger");
    await goCacheTrigger.click();
    const cleanButton = testPage.getByTestId("storage-go-cache-clean");
    await expect(cleanButton).toBeVisible();
    await cleanButton.focus();
    await expect(cleanButton).toBeFocused();

    setSnapshot(
      storageBarsSnapshot({
        workspaces: 1 * 1024 ** 3,
        systemTemporary: 20 * 1024 ** 3,
        goCache: 16 * 1024 ** 3,
        goCacheWarning: "cache measurement warning",
      }),
    );
    const heldRefresh = holdNextOverviewRefresh();
    await expect(cleanButton).toBeFocused();
    await testPage.getByTestId("storage-analyze").click();
    await heldRefresh.started;
    await cleanButton.focus();
    await expect(cleanButton).toBeFocused();
    heldRefresh.release();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
    );
    await expect
      .poll(() => resourceOrder(testPage))
      .toEqual([
        "system-temporary",
        "go-cache",
        "database",
        "database-backups",
        "unmanaged-go-cache",
        "quarantine",
        "temporary-artifacts",
        "managed-containers",
        "workspaces",
        "docker-image-layers",
        "docker-build-cache",
        "docker-unused-images",
        "docker-networks",
      ]);
    await expectResourceExpanded(testPage, "system-temporary");
    await expectResourceExpanded(testPage, "workspaces");
    await expectResourceExpanded(testPage, "go-cache");
    await expect(cleanButton).toBeFocused();
    expect(cleanupRequests).toHaveLength(0);
  });
});
