import { expect, test } from "../../fixtures/test-base";
import { enableCanvasFeature } from "./canvas-fixture";

const CANVAS_ENTRY = {
  id: "canvas-mobile-board",
  kind: "canvas",
  name: "Mobile board",
  description: "A portable board with a focused phone detail view.",
  author: "Kandev E2E",
  categories: ["planning"],
  icon_url: "",
  repo_url: "https://github.com/example/mobile-board",
  version: "1.0.0",
  min_kandev_version: "0.94.0",
  license: "MIT",
  package_url: "https://downloads.example.test/canvas-mobile-board-1.0.0.tar.gz",
  package_sha256: "b".repeat(64),
  previews: [
    {
      url: "https://images.example.test/mobile-board.png",
      alt: "Mobile board with a compact task list",
    },
  ],
  permissions: { reads: ["tasks"], writes: [], events: [], shared_state: false },
  stars: 4,
  updated_at: "2026-09-01T00:00:00.000Z",
  install_state: "available",
  source_id: "official",
  source_name: "Kandev Official",
};

async function mockCanvasCatalog(page: import("@playwright/test").Page) {
  await page.route("https://images.example.test/**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "image/png",
      body: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
        "base64",
      ),
    });
  });
  await page.route("**/api/plugins/marketplace*", async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        plugins: [],
        canvases: [CANVAS_ENTRY],
        sources: [
          {
            id: "official",
            name: "Kandev Official",
            url: "https://registry.example.test/index.json",
            enabled: true,
            builtin: true,
            healthy: true,
          },
        ],
      }),
    });
  });
}

test.describe("Canvas marketplace on mobile", () => {
  test("uses one-column cards, focused details, and reachable install controls", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    try {
      await mockCanvasCatalog(testPage);
      await testPage.goto("/settings/plugins");
      await testPage.getByTestId("plugins-tab-canvases").tap();

      const card = testPage.getByTestId(`canvas-marketplace-entry-${CANVAS_ENTRY.id}`);
      await expect(card).toBeVisible();
      expect((await card.boundingBox())?.width).toBeGreaterThan(280);
      await card.getByRole("button").first().tap();

      const detail = testPage.getByTestId(`canvas-marketplace-detail-${CANVAS_ENTRY.id}`);
      await expect(detail).toBeVisible();
      const install = detail.getByRole("button", { name: "Review and install", exact: true });
      await expect(install).toBeVisible();
      expect((await install.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);

      await install.tap();
      const dialog = testPage.getByRole("dialog").last();
      await expect(dialog.getByText("Preview images")).toBeVisible();
      expect(
        (await dialog.getByRole("button", { name: "Cancel", exact: true }).boundingBox())?.height,
      ).toBeGreaterThanOrEqual(44);
    } finally {
      await releaseFeature();
    }
  });
});
