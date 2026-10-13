import { test } from "../../fixtures/test-base";
import {
  exerciseDeliveryRecordRecovery,
  exerciseLiveDeliveryRecordBlock,
} from "../../helpers/delivery-record-recovery";

test("repairs missing delivery records without resending a prompt", async ({
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
    mobile: false,
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
    mobile: false,
  });
});
