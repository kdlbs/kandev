import { test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  seedSelectedFilters,
  openSelectedFilters,
  verifySelectedFilters,
} from "./sidebar-filter-selected-first-helpers";

// @covers AC-UI-FILTER-SELECTED-FIRST-001.5
test("phone view filters surface selected options first through touch controls", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  const seeded = await seedSelectedFilters(apiClient, seedData, backend.tmpDir);
  const editor = await openSelectedFilters(testPage, seeded, true);
  await verifySelectedFilters(testPage, editor, seeded, true);
  await waitForFiniteAnimations(testPage.getByTestId("filter-value-multi-popover"));
  await testPage.screenshot({ path: testInfo.outputPath("selected-first-phone.png") });
});
