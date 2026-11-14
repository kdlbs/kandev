import { test, expect } from "../../fixtures/test-base";
import { withJiraWatchFixture } from "./jira-watch-dialog-ownership-fixture";
import { jiraWatchDialogFlow } from "./jira-watch-dialog-ownership-flow";

for (const create of [false, true]) {
  test(`Jira ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withJiraWatchFixture(apiClient, seedData, info, (fixture) =>
      jiraWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: false, create }),
    );
  });
}
test("owned fixture cleans resources after a deliberate flow failure", async ({
  apiClient,
  seedData,
}, info) => {
  await expect(
    withJiraWatchFixture(apiClient, seedData, info, async () => {
      throw new Error("deliberate cleanup control");
    }),
  ).rejects.toThrow("deliberate cleanup control");
});
