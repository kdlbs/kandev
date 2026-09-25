import { expect, test } from "../../fixtures/test-base";

test.describe("Token usage stats navigation", () => {
  test("switches between overview and token usage routes", async ({ testPage, seedData }) => {
    await testPage.goto(`/stats?workspaceId=${seedData.workspaceId}`);
    await expect(testPage.getByRole("button", { name: "Copy Stats", exact: true })).toBeVisible();

    await testPage.getByRole("tab", { name: "Token Usage", exact: true }).click();
    await expect(testPage).toHaveURL(/\/stats\/token-usage\?workspaceId=/);
    await expect(testPage.getByTestId("token-usage-topbar")).toBeVisible();

    await testPage.getByRole("tab", { name: "Overview", exact: true }).click();
    await expect(testPage).toHaveURL(/\/stats\?workspaceId=/);
    await expect(testPage.getByRole("button", { name: "Copy Stats", exact: true })).toBeVisible();
  });
});
