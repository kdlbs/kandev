import { test } from "../../fixtures/test-base";
import { verifyAutomaticRestartRecovery } from "../../helpers/automatic-restart-recovery";

test.describe("automatic restart recovery chat", () => {
  test.describe.configure({ retries: 0 });

  test("restores an ordinary interrupted session on startup", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(180_000);
    await verifyAutomaticRestartRecovery({
      initialPage: testPage,
      api: apiClient,
      seed: seedData,
      backend,
      recoveryCase: "ordinary-restart",
      captureFile: testInfo.file,
    });
  });

  test("restores after an earlier terminal incident", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(180_000);
    await verifyAutomaticRestartRecovery({
      initialPage: testPage,
      api: apiClient,
      seed: seedData,
      backend,
      recoveryCase: "historical-terminal",
      captureFile: testInfo.file,
    });
  });

  test("recovers a failed persisted session on the first upgraded startup", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(180_000);
    await verifyAutomaticRestartRecovery({
      initialPage: testPage,
      api: apiClient,
      seed: seedData,
      backend,
      recoveryCase: "failed-session-upgrade",
      captureFile: testInfo.file,
    });
  });
});
