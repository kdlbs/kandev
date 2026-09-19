import { test } from "../../fixtures/test-base";
import { runInitialTaskBriefFlow } from "./initial-task-brief-helpers";

test.describe("Initial task brief on mobile", () => {
  // @covers AC-TASKS-INITIAL-TASK-BRIEF-001.1, AC-TASKS-INITIAL-TASK-BRIEF-001.2, AC-TASKS-INITIAL-TASK-BRIEF-001.7
  test("keeps the brief in the first user message after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await runInitialTaskBriefFlow({
      testPage,
      apiClient,
      seedData,
      title: "Initial task brief mobile",
    });
  });
});
