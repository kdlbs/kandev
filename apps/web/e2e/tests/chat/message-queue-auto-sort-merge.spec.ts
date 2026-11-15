import { test } from "../../fixtures/test-base";
import { exerciseAutoSortMergeQueue } from "../../helpers/message-queue-auto-sort-merge";

test("inherited sorted admission, session compaction, and incompatible latest sender", async ({
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
