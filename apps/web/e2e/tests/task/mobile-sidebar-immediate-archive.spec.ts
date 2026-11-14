import { test } from "../../fixtures/test-base";
import { checkImmediateArchive } from "./sidebar-immediate-archive-helpers";
import {
  checkDelayedReplacementArchive,
  checkRapidIndependentArchives,
} from "./sidebar-rapid-archive-helpers";

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

test("phone picker keeps independent archive controls for rapid archives", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await testPage.setViewportSize({ width: 320, height: 568 });
  await checkRapidIndependentArchives({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: true,
    prCapture,
  });
});

test("phone picker never selects a pending archive as the replacement", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 320, height: 568 });
  await checkDelayedReplacementArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: true,
  });
});
