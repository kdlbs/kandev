import { test } from "../../fixtures/test-base";
import { checkImmediateArchive } from "./sidebar-immediate-archive-helpers";
import {
  checkDelayedReplacementArchive,
  checkRapidIndependentArchives,
} from "./sidebar-rapid-archive-helpers";

test("sidebar shows archive progress before removal and recovers on failure", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await checkImmediateArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: false,
    prCapture,
  });
});

test("sidebar keeps independent archive controls for rapid individual archives", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await checkRapidIndependentArchives({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: false,
    prCapture,
  });
});

test("sidebar never selects a candidate archived during a delayed read", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkDelayedReplacementArchive({
    page: testPage,
    api: apiClient,
    seed: seedData,
    mobile: false,
  });
});
