import { test } from "../../fixtures/test-base";
import { withJiraWatchFixture } from "./jira-watch-dialog-ownership-fixture";
import { jiraWatchDialogFlow } from "./jira-watch-dialog-ownership-flow";

for (const create of [false, true]) {
  test(`phone Jira ${create ? "create" : "update"} completion preserves a later opening`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withJiraWatchFixture(apiClient, seedData, info, (fixture) =>
      jiraWatchDialogFlow(testPage, apiClient, seedData, fixture, { mobile: true, create }),
    );
  });
}
