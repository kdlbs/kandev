import { test } from "../../fixtures/test-base";
import { verifyRepeatedNativeResume } from "../../helpers/repeated-native-resume";

test.describe("mobile: repeated native resume after silent restoration", () => {
  test.describe.configure({ retries: 0 });

  test("resolves matching duplicate fences without replaying historical work", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await verifyRepeatedNativeResume(testPage, apiClient, seedData, backend, "matching");
  });

  test("keeps an independent restriction blocked", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await verifyRepeatedNativeResume(testPage, apiClient, seedData, backend, "independent");
  });
});
