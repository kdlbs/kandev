import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  expectContained,
  expectPage,
  navigate,
  preparePagination,
  scrollToPagination,
  seedPaginationTasks,
} from "../../helpers/task-pagination";

// @covers AC-UI-TASK-PAGINATION-PRESENTATION-001.1 AC-UI-TASK-PAGINATION-PRESENTATION-001.3
test("desktop and narrow fine-pointer pagination retain direct navigation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await seedPaginationTasks(apiClient, seedData);
  await preparePagination(testPage, seedData.workspaceId, false);
  const footer = testPage.getByTestId("tasks-pagination");
  const next = footer.getByRole("button", { name: "Go to next page" });
  const first = footer.getByRole("button", { name: "Go to first page" });
  const last = footer.getByRole("button", { name: "Go to last page" });
  await expect(first).toBeVisible();
  await expect(first).toBeDisabled();
  for (const number of [2, 3, 4]) await navigate(testPage, next, number, false);
  await expectContained(testPage, 27);
  const pageButtons = await footer.locator('[data-slot="pagination-content"] button:visible').all();
  const tops = await Promise.all(
    pageButtons.map(async (button) => (await button.boundingBox())!.y),
  );
  expect(Math.max(...tops) - Math.min(...tops)).toBeLessThanOrEqual(1);
  await navigate(testPage, last, 7, false);
  await expect(last).toBeDisabled();
  await navigate(testPage, first, 1, false);
  const pageTwo = footer.getByRole("button", { name: "2", exact: true });
  await pageTwo.focus();
  const loaded = waitForHttp(testPage, "GET", /\/api\/v1\/workspaces\/[^/]+\/tasks$/, {
    predicate: (response) => new URL(response.url()).searchParams.get("page") === "2",
  });
  await testPage.keyboard.press("Enter");
  await loaded;
  await expectPage(testPage, 2);
  await testPage.setViewportSize({ width: 320, height: 852 });
  await scrollToPagination(testPage);
  await navigate(testPage, next, 3, false);
  await navigate(testPage, next, 4, false);
  await expectContained(testPage, 44);
});
