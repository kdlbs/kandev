import { test, expect } from "../../fixtures/test-base";
import { withSentryWatchFixture } from "./sentry-watch-dialog-fixture";
import { sentryWatchDialogFlow } from "./sentry-watch-dialog-flow";

for (const create of [false, true]) {
  test(`Sentry ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withSentryWatchFixture(apiClient, seedData, info, (fixture) =>
      sentryWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: false, create }),
    );
  });
}
test("owned fixture cleans resources after a deliberate flow failure", async ({
  apiClient,
  seedData,
}, info) => {
  await expect(
    withSentryWatchFixture(apiClient, seedData, info, async () => {
      throw new Error("deliberate cleanup control");
    }),
  ).rejects.toThrow("deliberate cleanup control");
});
