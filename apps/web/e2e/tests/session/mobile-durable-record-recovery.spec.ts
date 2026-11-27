import { test } from "../../fixtures/test-base";
import {
  exerciseDeliveryRecordRecovery,
  exerciseLiveDeliveryRecordBlock,
} from "../../helpers/delivery-record-recovery";

test("keeps missing delivery record recovery reachable on a phone", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}, testInfo) => {
  await exerciseDeliveryRecordRecovery({
    page: testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
    testInfo,
    mobile: true,
  });
});

test("shows a new delivery block without reloading the chat", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  await exerciseLiveDeliveryRecordBlock({
    page: testPage,
    apiClient,
    seedData,
    backend,
    testInfo,
    mobile: true,
  });
});
