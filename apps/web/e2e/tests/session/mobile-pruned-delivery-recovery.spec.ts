import { test } from "../../fixtures/test-base";
import { exercisePrunedDeliveryRecovery } from "../../helpers/pruned-delivery-recovery";
test("mobile: restored pruned history preserves the phone composer across another restart", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, info) => {
  test.skip(
    process.platform !== "linux",
    "isolated original-process death proof is Linux-specific",
  );
  await exercisePrunedDeliveryRecovery({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend,
    info,
    mobile: true,
  });
});
