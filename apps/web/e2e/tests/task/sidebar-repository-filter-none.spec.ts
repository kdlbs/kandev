import { test } from "../../fixtures/test-base";
import { seedRepositoryNone, verifyRepositoryNone } from "./sidebar-repository-filter-none-helpers";

// @covers AC-UI-REPOSITORY-FILTER-NONE-001.2
// @covers AC-UI-REPOSITORY-FILTER-NONE-001.3
// @covers AC-UI-REPOSITORY-FILTER-NONE-001.5
// @covers AC-UI-REPOSITORY-FILTER-NONE-001.7
test("repository filters include None with named repositories and restore saved choices", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const seeded = await seedRepositoryNone(apiClient, seedData, backend.tmpDir);
  await verifyRepositoryNone(testPage, apiClient, seeded, false);
});
