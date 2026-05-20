import { test, expect } from "../../fixtures/office-fixture";

test.describe("Projects", () => {
  test("projects page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/projects");
    await expect(testPage.getByRole("heading", { name: /Projects/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  // Onboarding intentionally no longer seeds a default project — per
  // `internal/office/onboarding/service.go`, projects are created on
  // demand by the user or the coordinator agent. The fixture's
  // `officeSeed.projectId` is therefore empty by design; this test
  // is skipped until that contract changes again.
  test.skip("onboarding creates a default project", async ({ officeApi: _, officeSeed }) => {
    expect(officeSeed.projectId).toBeDefined();
    expect(officeSeed.projectId.length).toBeGreaterThan(0);
  });
});
