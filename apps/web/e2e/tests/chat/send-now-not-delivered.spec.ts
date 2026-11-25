import { test } from "../../fixtures/test-base";
import { exerciseSendNowNotDelivered } from "../../helpers/send-now-not-delivered";

test("marks an unrestorable Send Now message not delivered", async ({
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
    mobile: false,
  });
});
