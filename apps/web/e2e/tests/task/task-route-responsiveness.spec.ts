import { test } from "../../fixtures/test-base";
import { assertImmediateTaskReturn } from "./task-route-responsiveness-helpers";

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7
test("selected task and cached conversation remain usable before route refresh completes", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  await assertImmediateTaskReturn(testPage, apiClient, seedData, backend, false);
});
