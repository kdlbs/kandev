import { test } from "../../fixtures/test-base";
import { exerciseRepositoryCollapse } from "./sidebar-collapse-loading-fixtures";

test("repository disclosure preserves desktop rows while its bounded page refreshes", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await exerciseRepositoryCollapse(testPage, apiClient, seedData, "desktop", prCapture);
});
