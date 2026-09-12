import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { assertPreparationAttachments } from "../../helpers/preparation-attachments";

useRegularMode();

for (const failPreparation of [false, true]) {
  test(`initial attachments survive preparation ${failPreparation ? "setup error" : "completion"}`, async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }, testInfo) => {
    test.setTimeout(180_000);
    await assertPreparationAttachments({
      testPage,
      apiClient,
      backend,
      seedData,
      testInfo,
      failPreparation,
    });
  });
}
