import { test, expect } from "../../fixtures/office-fixture";

test.describe("Inbox", () => {
  test("inbox starts empty", async ({ officeApi, officeSeed }) => {
    const inbox = await officeApi.getInbox(officeSeed.workspaceId);
    expect(inbox).toBeDefined();
  });

  test("approvals list is initially empty", async ({ officeApi, officeSeed }) => {
    const approvals = await officeApi.listApprovals(officeSeed.workspaceId);
    expect(approvals).toBeDefined();
  });

  test("inbox page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/inbox");
    await expect(testPage.getByRole("heading", { name: /inbox/i })).toBeVisible({
      timeout: 10_000,
    });
  });
});
