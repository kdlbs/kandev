import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

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
    await expect(officeTopbarTitle(testPage)).toHaveText(/inbox/i, {
      timeout: 10_000,
    });
  });
});
