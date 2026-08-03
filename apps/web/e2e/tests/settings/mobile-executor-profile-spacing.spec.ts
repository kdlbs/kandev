import { test } from "../../fixtures/test-base";
import {
  expectExecutorProfileCardsSeparated,
  expectNoHorizontalOverflow,
} from "./executor-profile-spacing-helpers";

test.describe("executor profile card spacing on mobile", () => {
  test("keeps edit and create cards separated without horizontal overflow", async ({
    seedData,
    testPage,
  }) => {
    await testPage.goto(`/settings/executors/${seedData.worktreeExecutorProfileId}`);
    await expectExecutorProfileCardsSeparated(testPage);
    await expectNoHorizontalOverflow(testPage);

    await testPage.goto("/settings/executors/new/worktree");
    await expectExecutorProfileCardsSeparated(testPage);
    await expectNoHorizontalOverflow(testPage);
  });
});
