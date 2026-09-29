import { test } from "../../fixtures/test-base";
import { assertImmediateTaskReturn } from "./task-route-responsiveness-helpers";

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7
test("phone task picker shows cached conversation before route refresh completes", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  await assertImmediateTaskReturn(testPage, apiClient, seedData, backend, true);
});
