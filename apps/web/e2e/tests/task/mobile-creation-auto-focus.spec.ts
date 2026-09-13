import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { verifyCreationAutoFocus } from "./creation-auto-focus-helpers";

useRegularMode();
test.setTimeout(180_000);
test("keeps mobile creation in context with a saved touch-accessible preference", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  expect(testPage.viewportSize()).toEqual(testInfo.project.use.viewport);
  expect(await testPage.evaluate(() => navigator.maxTouchPoints)).toBeGreaterThan(0);
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  await verifyCreationAutoFocus(testPage, apiClient, seedData.workspaceId, true);
});
