import { test } from "../../fixtures/test-base";
import { exerciseIncrementalShellOutput } from "../../helpers/incremental-shell-output";

for (const exitCode of [7, null]) {
  test(`incremental shell output survives reload and final exit ${exitCode}`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    await exerciseIncrementalShellOutput({
      testPage,
      apiClient,
      seedData,
      backend,
      touch: false,
      exitCode,
    });
  });
}

test("cancelled incremental shell output survives reload", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseIncrementalShellOutput({
    testPage,
    apiClient,
    seedData,
    backend,
    touch: false,
    exitCode: null,
    cancel: true,
  });
});
