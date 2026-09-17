import { test } from "../../fixtures/test-base";
import { checkImmediateArchive } from "./sidebar-immediate-archive-helpers";

test("phone picker shows archive progress and restores failed archive", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkImmediateArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: true,
    screenshotPath: testInfo.outputPath("pending-archive.png"),
  });
});
