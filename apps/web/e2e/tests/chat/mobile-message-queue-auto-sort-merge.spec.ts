import { test } from "../../fixtures/test-base";
import { exerciseAutoSortMergeQueue } from "../../helpers/message-queue-auto-sort-merge";

test("mobile Auto-merge sorts by default and tapping it on compacts the pending queue", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(150_000);
  await exerciseAutoSortMergeQueue({
    page: testPage,
    apiClient,
    seedData,
    tmpDir: backend.tmpDir,
    mobile: true,
  });
});
