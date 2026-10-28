import { test } from "../../fixtures/test-base";
import { checkDownload, checkGeometry, openDownloadPlan } from "./plan-download-helpers";

test("downloads the current Plan draft on desktop", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await openDownloadPlan(testPage, apiClient, seedData, false);
  await checkGeometry(testPage, false);
  await prCapture.screenshot("plan-download-desktop", {
    caption: "Download the current Plan draft on desktop.",
  });
  await checkDownload(testPage, false);
});

test("keeps the Plan actions contained on a narrow fine-pointer phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 375, height: 812 });
  await openDownloadPlan(testPage, apiClient, seedData, true);
  await checkGeometry(testPage, true);
});
