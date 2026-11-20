import { test } from "./linear-watch-dialog-test";
import { withLinearWatchFixture } from "./linear-watch-dialog-fixture";
import { linearWatchDialogFlow } from "./linear-watch-dialog-flow";

for (const create of [false, true]) {
  test(`phone Linear ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withLinearWatchFixture(apiClient, seedData, info, (fixture) =>
      linearWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: true, create }),
    );
  });
}
