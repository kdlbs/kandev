import { test } from "../../fixtures/test-base";
import { checkDownload, checkGeometry, openDownloadPlan } from "./plan-download-helpers";

test("downloads the current Plan draft on a phone", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await openDownloadPlan(testPage, apiClient, seedData, true);
  await checkGeometry(testPage, true);
  await prCapture.screenshot("plan-download-phone", {
    caption: "Download the current Plan draft on a phone.",
  });
  await checkDownload(testPage, true);
});

test("keeps the download control touch-sized on a coarse-pointer workbench", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await openDownloadPlan(testPage, apiClient, seedData, false);
  await checkGeometry(testPage, true);
});
