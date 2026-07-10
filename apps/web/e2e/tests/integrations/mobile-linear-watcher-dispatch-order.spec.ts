import { test } from "../../fixtures/test-base";
import { assertWatcherDispatchOrderPersists } from "./watcher-dispatch-order-flow";

test.describe("Linear watcher dispatch order (mobile)", () => {
  test("persists the selected dispatch order across save and reopen", async ({
    testPage,
    apiClient,
  }) => {
    await assertWatcherDispatchOrderPersists(testPage, apiClient);
  });
});
