import { test, expect } from "../../fixtures/test-base";
import {
  seedSelectedFilters,
  openSelectedFilters,
  verifySelectedFilters,
} from "./sidebar-filter-selected-first-helpers";

// @covers AC-UI-FILTER-SELECTED-FIRST-001.1
// @covers AC-UI-FILTER-SELECTED-FIRST-001.2
// @covers AC-UI-FILTER-SELECTED-FIRST-001.3
// @covers AC-UI-FILTER-SELECTED-FIRST-001.4
test("view filters prioritize selections across repositories and workflows", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const seeded = await seedSelectedFilters(apiClient, seedData, backend.tmpDir);
  const editor = await openSelectedFilters(testPage, seeded, false);
  await verifySelectedFilters(testPage, editor, seeded, false);
  const { settings } = await apiClient.getUserSettings();
  expect(settings.sidebar_views_by_workspace[seedData.workspaceId].views[0].filters).toEqual(
    seeded.filters,
  );
});
