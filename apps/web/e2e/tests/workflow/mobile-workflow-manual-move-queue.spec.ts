import { test } from "../../fixtures/test-base";
import {
  expectNoQueuePanelHorizontalOverflow,
  expectWorkflowQueueBadge,
  seedQueuedWorkflowMessageScenario,
} from "./workflow-queue-helpers";

test.describe("Workflow queued messages on mobile", () => {
  test("manual move queue badge stays usable on narrow screens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedQueuedWorkflowMessageScenario(
      testPage,
      apiClient,
      seedData,
      "Mobile queued workflow",
    );

    await expectWorkflowQueueBadge(session);
    await expectNoQueuePanelHorizontalOverflow(testPage);
  });
});
