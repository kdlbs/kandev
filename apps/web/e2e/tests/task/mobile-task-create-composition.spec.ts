import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { COMPOSITION_CASES, verifyTaskCreateComposition } from "./task-create-composition-helpers";

useRegularMode();

// @covers AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.2, .4, .5
for (const signal of COMPOSITION_CASES) {
  test(`phone Create Task ignores synthetic ${signal.name} and accepts an ordinary chord`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await verifyTaskCreateComposition({
      page: testPage,
      api: apiClient,
      workspaceId: seedData.workspaceId,
      capture: prCapture,
      mobile: true,
      signal,
    });
  });
}

test("phone Create Task retains visible touch submission", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await verifyTaskCreateComposition({
    page: testPage,
    api: apiClient,
    workspaceId: seedData.workspaceId,
    capture: prCapture,
    mobile: true,
  });
});
