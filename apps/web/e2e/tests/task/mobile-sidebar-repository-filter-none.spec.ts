import { test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  openRepositoryNone,
  seedRepositoryNone,
  verifyRepositoryNone,
} from "./sidebar-repository-filter-none-helpers";

// @covers AC-UI-REPOSITORY-FILTER-NONE-001.7
test("phone repository filters include None through the task-view picker", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  const seeded = await seedRepositoryNone(apiClient, seedData, backend.tmpDir);
  await verifyRepositoryNone(testPage, apiClient, seeded, true);
  const editor = await openRepositoryNone(testPage, seeded, true);
  await editor.getByTestId("filter-value-multi").tap();
  await waitForFiniteAnimations(testPage.getByTestId("filter-value-multi-popover"));
  await testPage.screenshot({ path: testInfo.outputPath("repository-none-phone.png") });
});
