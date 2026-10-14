import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test("inbox and approval endpoints serve the Inbox page", async ({
  testPage,
  officeApi,
  officeSeed,
}) => {
  const inbox = (await officeApi.getInbox(officeSeed.workspaceId)) as {
    items: unknown[];
    total_count: number;
  };
  expect(Array.isArray(inbox.items)).toBe(true);
  expect(inbox.total_count).toBe(inbox.items.length);
  const approvals = (await officeApi.listApprovals(officeSeed.workspaceId)) as {
    approvals: unknown[];
  };
  expect(Array.isArray(approvals.approvals)).toBe(true);
  await testPage.goto("/office/inbox");
  await expect(officeTopbarTitle(testPage)).toHaveText("Inbox", { timeout: 10_000 });
  await expect(testPage.getByRole("tab", { name: "Mine" })).toBeVisible();
});
