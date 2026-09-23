import { test } from "../../fixtures/test-base";
import { checkImmediateArchive } from "./sidebar-immediate-archive-helpers";

test("phone picker shows archive progress and restores failed archive", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await testPage.setViewportSize({ width: 320, height: 568 });
  await checkImmediateArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: true,
    prCapture,
  });
});
