import { test, expect } from "../../fixtures/office-fixture";

test.describe("Routines UI", () => {
  test("routine created via API appears in page", async ({ testPage, officeApi, officeSeed }) => {
    await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Test Routine",
    });
    await testPage.goto("/office/routines");
    await expect(testPage.getByText("E2E Test Routine")).toBeVisible({ timeout: 10_000 });
  });
});
