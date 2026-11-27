import { test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import {
  delayedCapacitySuccess,
  delayedCapacityExpiry,
  delayedCapacityCancellation,
  delayedCapacityRestart,
} from "../../helpers/delayed-capacity-retries";

test("phone recovers after the fifth capacity retry with reachable Cancel", async ({
  testPage,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  await delayedCapacitySuccess(testPage, backend, apiClient, seedData, {
    mobile: true,
    capture: prCapture,
  });
});
test("phone expires capacity recovery and preserves a usable composer", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}, testInfo) => {
  const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "expiry" });
  await delayedCapacityExpiry(testPage, backend, apiClient, seedData, capture);
  capture.flush();
});
test("phone cancels delayed capacity recovery and accepts new input", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  await delayedCapacityCancellation(testPage, backend, apiClient, seedData, true);
});

test("phone retires delayed capacity recovery after restart", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  await delayedCapacityRestart(testPage, backend, apiClient, seedData);
});
