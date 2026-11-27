import { test } from "../../fixtures/test-base";
import { exerciseRepositoryCollapse } from "./sidebar-collapse-loading-fixtures";

test("repository disclosure preserves phone picker and navigation rows during refresh", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  for (const surface of ["picker", "navigation"] as const) {
    await exerciseRepositoryCollapse(testPage, apiClient, seedData, surface, prCapture);
  }
});
