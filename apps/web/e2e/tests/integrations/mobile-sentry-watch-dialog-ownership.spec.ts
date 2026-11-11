import { test } from "../../fixtures/test-base";
import { withSentryWatchFixture } from "./sentry-watch-dialog-fixture";
import { sentryWatchDialogFlow } from "./sentry-watch-dialog-flow";

for (const create of [false, true]) {
  test(`phone Sentry ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withSentryWatchFixture(apiClient, seedData, info, (fixture) =>
      sentryWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: true, create }),
    );
  });
}
