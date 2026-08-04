import { test } from "../../fixtures/test-base";
import { expectExecutorProfileCardsSeparated } from "./executor-profile-spacing-helpers";

test.describe("executor profile card spacing", () => {
  test("separates cards on edit and create pages", async ({ seedData, testPage }) => {
    await testPage.goto(`/settings/executors/${seedData.worktreeExecutorProfileId}`);
    await expectExecutorProfileCardsSeparated(testPage);

    await testPage.goto("/settings/executors/new/worktree");
    await expectExecutorProfileCardsSeparated(testPage);
  });
});
