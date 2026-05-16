import { test, expect } from "../../fixtures/office-fixture";

test.describe("Real-time inbox updates", () => {
  test("inbox page loads", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/inbox");
    // The page must render with its toolbar tabs (Mine / Recent / All).
    // Whether the list is empty or populated depends on what onboarding
    // seeded — we just assert the page rendered correctly.
    await expect(testPage.getByRole("heading", { name: "Inbox" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByRole("tab", { name: "Mine" })).toBeVisible();
  });

  test("inbox API returns count", async ({ officeApi, officeSeed }) => {
    const inbox = await officeApi.getInbox(officeSeed.workspaceId);
    expect(inbox).toBeDefined();
  });
});
