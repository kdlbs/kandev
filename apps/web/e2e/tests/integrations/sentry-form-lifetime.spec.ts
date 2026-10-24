import { test } from "../../fixtures/test-base";
import { exerciseSentryFormLifetime } from "./sentry-form-lifetime-helpers";

for (const kind of ["add", "edit"] as const) {
  test(`preserves the current Sentry form after an earlier ${kind} completes`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.expect(seedData.worktreeExecutorProfileId).toBeTruthy();
    await exerciseSentryFormLifetime(testPage, apiClient, prCapture, { touch: false, kind });
  });
}
