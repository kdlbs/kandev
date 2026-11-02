import { test, expect } from "./linear-watch-dialog-test";
import { withLinearWatchFixture } from "./linear-watch-dialog-fixture";
import { linearWatchDialogFlow } from "./linear-watch-dialog-flow";

for (const create of [false, true]) {
  test(`Linear ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withLinearWatchFixture(apiClient, seedData, info, (fixture) =>
      linearWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: false, create }),
    );
  });
}
test("owned fixture cleans resources after a deliberate flow failure", async ({
  apiClient,
  seedData,
}, info) => {
  await expect(
    withLinearWatchFixture(apiClient, seedData, info, async () => {
      throw new Error("deliberate cleanup control");
    }),
  ).rejects.toThrow("deliberate cleanup control");
});
