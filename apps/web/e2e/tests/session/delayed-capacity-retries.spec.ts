import { test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import {
  delayedCapacitySuccess,
  delayedCapacityExpiry,
  delayedCapacityCancellation,
  delayedCapacityRestart,
} from "../../helpers/delayed-capacity-retries";

test("desktop recovers after the fifth capacity retry without repeating completed work", async ({
  testPage,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  await delayedCapacitySuccess(testPage, backend, apiClient, seedData, {
    mobile: false,
    capture: prCapture,
  });
});
test("desktop expires capacity recovery without another dispatch", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}, testInfo) => {
  const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "expiry" });
  await delayedCapacityExpiry(testPage, backend, apiClient, seedData, capture);
  capture.flush();
});
test("desktop cancels delayed capacity recovery and accepts new input", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  await delayedCapacityCancellation(testPage, backend, apiClient, seedData, false);
});

test("desktop retires delayed capacity recovery after restart", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  await delayedCapacityRestart(testPage, backend, apiClient, seedData);
});
