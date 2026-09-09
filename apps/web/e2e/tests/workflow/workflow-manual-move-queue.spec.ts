import { test } from "../../fixtures/test-base";
import {
  expectDeliveredWorkflowMessage,
  expectWorkflowQueueBadge,
  seedQueuedWorkflowMessageScenario,
} from "./workflow-queue-helpers";

test.describe("Workflow queued messages", () => {
  test("manual move during an active turn queues a workflow step message", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, sessionId } = await seedQueuedWorkflowMessageScenario(
      testPage,
      apiClient,
      seedData,
      "Desktop queued workflow",
    );

    await expectWorkflowQueueBadge(session);
    await expectDeliveredWorkflowMessage(apiClient, session, sessionId);
  });
});
