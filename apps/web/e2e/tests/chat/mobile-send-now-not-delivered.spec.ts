import { test } from "../../fixtures/test-base";
import { exerciseSendNowNotDelivered } from "../../helpers/send-now-not-delivered";

test("shows the not-delivered Send Now badge on a phone", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}, testInfo) => {
  await exerciseSendNowNotDelivered({
    page: testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
    testInfo,
    mobile: true,
  });
});
