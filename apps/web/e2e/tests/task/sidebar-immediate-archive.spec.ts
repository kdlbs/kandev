import { test } from "../../fixtures/test-base";
import { checkImmediateArchive } from "./sidebar-immediate-archive-helpers";

test("sidebar archive disappears before server processing and recovers on failure", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkImmediateArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: false,
    screenshotPath: testInfo.outputPath("pending-archive.png"),
  });
});
