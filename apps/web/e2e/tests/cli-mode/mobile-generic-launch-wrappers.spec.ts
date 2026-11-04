import { test } from "../../fixtures/test-base";
import { checkWrappedPassthrough } from "../../helpers/generic-wrapper-passthrough";

test.use({ trace: "retain-on-failure" });

test("wraps native terminal input, resume, and cancellation on a phone", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(180_000);
  await checkWrappedPassthrough(testPage, apiClient, backend, seedData, true);
});
