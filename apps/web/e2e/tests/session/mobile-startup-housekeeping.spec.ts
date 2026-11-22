import { test } from "../../fixtures/test-base";
import { verifyStartupHousekeeping } from "../../helpers/startup-housekeeping";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test("restores the selected phone conversation after startup housekeeping without sending an instruction", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180000);
  await verifyStartupHousekeeping(testPage, apiClient, seedData, backend);
  await assertNoDocumentHorizontalOverflow(testPage, "restored phone conversation");
});
