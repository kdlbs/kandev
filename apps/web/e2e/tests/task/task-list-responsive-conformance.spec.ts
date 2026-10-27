import fs from "node:fs/promises";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";

const EVIDENCE_DIR = path.resolve(
  __dirname,
  "../../../../../docs/plans/mobile-list-layout/evidence",
);

async function captureResponsiveState(
  page: import("@playwright/test").Page,
  testInfo: import("@playwright/test").TestInfo,
  name: string,
) {
  await waitForFiniteAnimations(page.locator("body"));
  const outputPath = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path: outputPath, animations: "disabled", caret: "hide" });
  const geometry = await page.evaluate(() => ({
    viewport: { width: window.innerWidth, height: window.innerHeight },
    pointer: {
      fine: matchMedia("(pointer: fine)").matches,
      coarse: matchMedia("(pointer: coarse)").matches,
    },
    document: {
      width: document.documentElement.scrollWidth,
      height: document.documentElement.scrollHeight,
      horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth,
    },
    elements: Object.fromEntries(
      [
        "mobile-topbar-page-context",
        "mobile-list-search-toggle",
        "app-nav-trigger",
        "sidebar-view-chip-row",
        "app-sidebar",
        "tasks-list-search",
        "tasks-list-archive-modes",
        "tasks-list-sort",
        "tasks-list-group",
        "threads-board",
        "threads-view-controls",
      ].map((testId) => {
        const element = document.querySelector(`[data-testid="${testId}"]`);
        if (!element) return [testId, null];
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return [
          testId,
          {
            x: rect.x,
            y: rect.y,
            width: rect.width,
            height: rect.height,
            minHeight: style.minHeight,
            overflowX: style.overflowX,
            overflowY: style.overflowY,
            clientHeight: element.clientHeight,
            scrollHeight: element.scrollHeight,
            insideViewport:
              rect.left >= 0 &&
              rect.top >= 0 &&
              rect.right <= window.innerWidth &&
              rect.bottom <= window.innerHeight,
          },
        ];
      }),
    ),
  }));
  const geometryPath = testInfo.outputPath(`${name}.json`);
  await fs.writeFile(geometryPath, `${JSON.stringify(geometry, null, 2)}\n`);
  const capturePrAssets = process.env.CAPTURE_PR_ASSETS === "true";
  if (capturePrAssets) {
    await fs.mkdir(EVIDENCE_DIR, { recursive: true });
    await fs.copyFile(outputPath, path.join(EVIDENCE_DIR, `${name}.png`));
    await fs.copyFile(geometryPath, path.join(EVIDENCE_DIR, `${name}.json`));
  }
  await testInfo.attach(name, { path: outputPath, contentType: "image/png" });
  await testInfo.attach(`${name}-geometry`, {
    path: geometryPath,
    contentType: "application/json",
  });
  expect(geometry.document.horizontalOverflow).toBe(false);
  console.log(
    `VISUAL_ACCEPTANCE ${JSON.stringify({ name, path: capturePrAssets ? EVIDENCE_DIR : geometryPath, ...geometry })}`,
  );
}

test("captures fine-pointer phone composition and desktop List/Threads compatibility", async ({
  testPage,
}, testInfo) => {
  await testPage.setViewportSize({ width: 393, height: 852 });
  await testPage.goto("/tasks");
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();
  await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-search")).toHaveCount(0);
  await captureResponsiveState(testPage, testInfo, "ui-01-phone-fine-pointer-393x852");

  await testPage.setViewportSize({ width: 767, height: 900 });
  await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();
  await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
  await captureResponsiveState(testPage, testInfo, "ui-01-phone-fine-breakpoint-767x900");

  await testPage.setViewportSize({ width: 1024, height: 900 });
  const sidebar = testPage.getByTestId("app-sidebar");
  await expect(sidebar).toBeVisible();
  if ((await sidebar.getAttribute("data-collapsed")) === "true") {
    await sidebar.locator("[data-sidebar-toggle]").click();
  }
  const tasksSection = sidebar.locator('[aria-controls="sidebar-section-tasks"]').first();
  if ((await tasksSection.getAttribute("aria-expanded")) !== "true") await tasksSection.click();
  await expect(sidebar.getByTestId("tasks-view-picker")).toBeVisible();
  await expect(sidebar.getByTestId("sidebar-filter-gear")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-view")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-search")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-archive-modes")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-sort")).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-group")).toBeVisible();
  await captureResponsiveState(testPage, testInfo, "ui-07-desktop-list-fine-1024x900");

  await testPage.goto("/threads");
  await expect(testPage.getByTestId("threads-view-controls")).toBeVisible();
  await expect
    .poll(async () => {
      const board = await testPage.getByTestId("threads-board").count();
      const emptyState = await testPage.getByTestId("threads-empty-state").count();
      return board + emptyState;
    })
    .toBeGreaterThan(0);
  await captureResponsiveState(testPage, testInfo, "ui-07-desktop-threads-fine-1024x900");
});
