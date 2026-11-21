import { test } from "../../fixtures/test-base";
import { exerciseAutoSortMergeQueue } from "../../helpers/message-queue-auto-sort-merge";

test("default Auto-merge sorts sender messages, session enable compacts, and global changes do not", async ({
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
    mobile: false,
  });
});
