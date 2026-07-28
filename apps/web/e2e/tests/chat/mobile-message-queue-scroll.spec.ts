import { test } from "../../fixtures/test-base";
import { expectFullQueueScrolls, seedFullQueueTask } from "./message-queue-scroll-helpers";

test("mobile full queue scrolls internally without hiding the composer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedFullQueueTask(
    testPage,
    apiClient,
    seedData,
    "Mobile full queue scrolling",
  );

  await expectFullQueueScrolls(session);
});
