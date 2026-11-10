import { test, expect } from "../../fixtures/test-base";
import {
  expectContained,
  expectPage,
  navigate,
  preparePagination,
  scrollToPagination,
  seedPaginationTasks,
} from "../../helpers/task-pagination";

// @covers AC-UI-TASK-PAGINATION-PRESENTATION-001.1 AC-UI-TASK-PAGINATION-PRESENTATION-001.2
for (const width of [320, 393, 430]) {
  test(`phone pagination remains reachable at ${width}px`, async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    await testPage.setViewportSize({ width, height: 852 });
    await seedPaginationTasks(apiClient, seedData);
    await preparePagination(testPage, seedData.workspaceId, true);
    const footer = testPage.getByTestId("tasks-pagination");
    const next = footer.getByRole("button", { name: "Go to next page" });
    await expect(footer.getByRole("button", { name: "Go to previous page" })).toBeDisabled();
    await expectContained(testPage, 44);
    for (const number of [2, 3, 4]) await navigate(testPage, next, number, true);
    await scrollToPagination(testPage);
    await testPage.screenshot({ path: testInfo.outputPath(`footer-${width}.png`) });
    await expectContained(testPage, 44);
    await navigate(testPage, next, 5, true);
    await navigate(testPage, footer.getByRole("button", { name: "Go to previous page" }), 4, true);
    await navigate(testPage, footer.getByRole("button", { name: "3", exact: true }), 3, true);
    await navigate(testPage, next, 4, true);
    for (const breakpointWidth of [640, 767, 768, 1023, 1024]) {
      await testPage.setViewportSize({ width: breakpointWidth, height: 852 });
      await expectPage(testPage, 4);
      await expectContained(testPage, 44);
      if (breakpointWidth === 1024) {
        await expect(footer.getByRole("button", { name: "Go to first page" })).toBeVisible();
        await expect(footer.getByRole("button", { name: "Go to last page" })).toBeVisible();
      }
    }
  });
}
