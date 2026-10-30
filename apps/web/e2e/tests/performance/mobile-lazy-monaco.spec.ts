import { test, resetSeedRepositoryCheckout } from "../../fixtures/test-base";
import { verifyLazyMonaco } from "../../helpers/lazy-monaco";

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.2
test("opens the native phone viewer without loading Monaco", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}, info) => {
  test.setTimeout(90_000);
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  await verifyLazyMonaco(testPage, apiClient, seedData, backend, { info, mobile: true });
  await prCapture.screenshot("phone-viewer");
});

test.afterEach(async ({ seedData, backend }) => {
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
});
