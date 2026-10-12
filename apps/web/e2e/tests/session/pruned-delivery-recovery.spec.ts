import { test } from "../../fixtures/test-base";
import { exercisePrunedDeliveryRecovery } from "../../helpers/pruned-delivery-recovery";
test("pruned history restores the original conversation and remains usable after another restart", async ({
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
    mobile: false,
  });
});
