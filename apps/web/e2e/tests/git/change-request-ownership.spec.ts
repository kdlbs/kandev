import { test } from "../../fixtures/test-base";
import { preserveCreationDraft } from "./change-request-ownership-helpers";

test("native Changes preserves a reopened creation draft after old success", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, info) => {
  test.setTimeout(180_000);
  await preserveCreationDraft(testPage, apiClient, seedData, { backend, info, mobile: false });
});
