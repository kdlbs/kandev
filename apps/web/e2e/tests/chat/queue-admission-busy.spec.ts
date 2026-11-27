import { test } from "../../fixtures/test-base";
import { registerSeparateQueueRows } from "../../helpers/message-queue-settings";
import { expectQueueBusyRetry } from "./queue-admission-busy.helpers";

registerSeparateQueueRows(test);
test.describe.configure({ retries: 0 });
test("queue busy feedback preserves entries and permits retry", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await expectQueueBusyRetry(testPage, apiClient, seedData, false, async () => {
    await testPage.screenshot({ path: testInfo.outputPath("queue-busy.png") });
  });
});
