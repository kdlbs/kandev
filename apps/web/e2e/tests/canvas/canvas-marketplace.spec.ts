import { expect, test } from "../../fixtures/test-base";
import { enableCanvasFeature } from "./canvas-fixture";

const CANVAS_ENTRY = {
  id: "canvas-release-board",
  kind: "canvas",
  name: "Release board",
  description: "A portable release board for workspace planning.",
  author: "Kandev E2E",
  categories: ["planning"],
  icon_url: "",
  repo_url: "https://github.com/example/release-board",
  version: "1.2.0",
  min_kandev_version: "0.94.0",
  license: "MIT",
  package_url: "https://downloads.example.test/canvas-release-board-1.2.0.tar.gz",
  package_sha256: "a".repeat(64),
  previews: [
    {
      url: "https://images.example.test/release-board-cover.png",
      alt: "Release board with grouped milestones",
    },
    {
      url: "https://images.example.test/release-board-detail.png",
      alt: "Release board milestone details",
    },
  ],
  permissions: {
    reads: ["tasks"],
    writes: [],
    events: ["task.updated"],
    shared_state: false,
    external_origins: [],
  },
  stars: 12,
  updated_at: "2026-09-01T00:00:00.000Z",
  install_state: "available",
  source_id: "official",
  source_name: "Kandev Official",
};

function canvasCatalog() {
  return {
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
  };
}

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
      body: JSON.stringify(canvasCatalog()),
    });
  });
}

test.describe("Canvas marketplace", () => {
  test("browses a registry canvas, opens details, and keeps manual install available", async ({
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
      await testPage.getByTestId("plugins-tab-canvases").click();

      const card = testPage.getByTestId(`canvas-marketplace-entry-${CANVAS_ENTRY.id}`);
      await expect(card).toBeVisible();
      await expect(card.locator(`img[alt="${CANVAS_ENTRY.previews[0].alt}"]`)).toBeVisible();
      await expect(testPage.getByTestId("canvas-marketplace-search")).toBeVisible();

      await card.getByRole("button").first().click();
      const detail = testPage.getByTestId(`canvas-marketplace-detail-${CANVAS_ENTRY.id}`);
      await expect(detail).toBeVisible();
      await expect(detail.getByLabel("Preview images")).toBeVisible();
      await expect(detail.getByText("tasks")).toBeVisible();

      await detail.getByRole("button", { name: "Review and install", exact: true }).click();
      const installDialog = testPage.getByRole("dialog").last();
      await expect(installDialog).toBeVisible();
      await expect(installDialog.getByText("Release board")).toBeVisible();
      await expect(
        installDialog.getByText("Preview images are registry metadata.", { exact: false }),
      ).toBeVisible();
      await expect(installDialog.getByText("Preview images")).toBeVisible();

      await installDialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await detail.getByRole("button", { name: "Back to canvases", exact: true }).click();
      await testPage
        .getByTestId("canvas-marketplace")
        .getByRole("button", { name: "Install canvas", exact: true })
        .first()
        .click();

      const manualDialog = testPage.getByRole("dialog").last();
      await expect(
        manualDialog.getByRole("button", { name: "Upload bundle", exact: true }),
      ).toBeVisible();
      await manualDialog.getByRole("button", { name: "Direct link", exact: true }).click();
      await expect(manualDialog.getByLabel("Direct link", { exact: true })).toBeVisible();
      await expect(manualDialog.getByText("Screenshot", { exact: false })).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });
});
