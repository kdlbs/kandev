import { test, expect } from "../../fixtures/office-fixture";

// Office-specific sidebar items (Inbox, Projects, Agents) must appear only while
// the user is inside the Office surface (any /office route, reached via the
// footer "Office" button) — not in the regular workspace, even with office on.
test.describe("Sidebar office gating", () => {
  test("office sections appear only inside Office", async ({ testPage, officeSeed }) => {
    expect(officeSeed.workspaceId).toBeTruthy();
    const sidebar = testPage.getByTestId("app-sidebar");

    // Inside Office: the office sections render.
    await testPage.goto("/office");
    await expect(sidebar.getByText("Projects", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(sidebar.getByText("Agents", { exact: true })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "Inbox" })).toBeVisible();

    // Regular workspace home: the office sections are gone.
    await testPage.goto("/");
    await expect(sidebar).toBeVisible();
    await expect(sidebar.getByText("Projects", { exact: true })).toHaveCount(0);
    await expect(sidebar.getByText("Agents", { exact: true })).toHaveCount(0);
    await expect(sidebar.getByRole("link", { name: "Inbox" })).toHaveCount(0);
  });
});
