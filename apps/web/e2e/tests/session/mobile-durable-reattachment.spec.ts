import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { verifyAutomaticRestartReconciliation } from "../../helpers/restart-reconciliation";

test.describe("mobile durable delivery reattachment", () => {
  test.describe.configure({ retries: 0 });
  test("restores an actual disconnect after reload on a phone", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    await verifyAutomaticRestartReconciliation(testPage, apiClient, seedData, backend);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile durable delivery reattachment");
  });
});
