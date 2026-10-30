import { test } from "../../fixtures/test-base";
import { verifyCanonicalStream } from "../../helpers/canonical-stream";

test("delivers committed agent stream updates without resetting the conversation", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, info) => {
  test.setTimeout(90_000);
  await verifyCanonicalStream(testPage, apiClient, seedData, backend, info);
});
