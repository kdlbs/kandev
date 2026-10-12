import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { assertPreparationAttachments } from "../../helpers/preparation-attachments";

useRegularMode();

test("mobile initial attachments survive preparation and reload", async ({
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
    mobile: true,
  });
});
