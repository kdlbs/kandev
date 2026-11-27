import { test } from "../../fixtures/test-base";
import { assertProgressiveNavigation } from "./task-navigation-helpers";

test.describe.configure({ retries: 0 });

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
test("phone Files remains usable through task chooser navigation and folder retry", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(90_000);
  await assertProgressiveNavigation(testPage, apiClient, seedData, backend, true);
});
