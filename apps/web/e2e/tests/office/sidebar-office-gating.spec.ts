import { test, expect } from "../../fixtures/office-fixture";

// Office-specific sidebar items (Inbox, Projects, Agents) follow the ACTIVE
// WORKSPACE, not the URL. They used to be gated on being under an /office
// route, which meant any shared surface — Stats, an integration dashboard, a
// task page — silently dropped an Office user back to kanban chrome.
test.describe("Sidebar office gating", () => {
  test("office sections follow the active workspace, not the route", async ({
    testPage,
    officeSeed,
    seedData,
  }) => {
    expect(officeSeed.workspaceId).toBeTruthy();
    const sidebar = testPage.getByTestId("app-sidebar");

    // An office workspace is active: the office sections render.
    await testPage.goto("/office");
    await expect(sidebar.getByText("Projects", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(sidebar.getByText("Agents", { exact: true })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "Inbox", exact: true })).toBeVisible();

    // A shared, non-office route with the same workspace still active. This is
    // the case the old rule got wrong: /stats is not an /office route, so the
    // sections used to disappear here.
    await testPage.goto("/stats");
    await expect(sidebar.getByText("Projects", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(sidebar.getByText("Agents", { exact: true })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "Inbox", exact: true })).toBeVisible();

    // Naming a kanban workspace in the URL switches the workspace, and the
    // chrome follows it.
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(sidebar).toBeVisible();
    await expect(sidebar.getByText("Projects", { exact: true })).toHaveCount(0);
    await expect(sidebar.getByText("Agents", { exact: true })).toHaveCount(0);
    await expect(sidebar.getByRole("link", { name: "Inbox", exact: true })).toHaveCount(0);
  });

  test("a Settings round trip keeps the active office workspace", async ({
    testPage,
    officeSeed,
    seedData,
  }) => {
    // Start on the kanban workspace and switch to the Office one through the
    // picker. The switch must be an explicit selection (picking the already
    // active workspace is a no-op): selection is what records the workspace
    // for later resolutions.
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${officeSeed.workspaceId}`).click();
    await expect(testPage).toHaveURL(
      (url) =>
        url.pathname === "/office" &&
        url.searchParams.get("workspaceId") === officeSeed.workspaceId,
      { timeout: 10_000 },
    );

    // Into Settings through the gear. Settings resolves an active workspace of
    // its own on entry, and it used to filter office workspaces out of that
    // resolution — so the exit below landed on some kanban board instead of the
    // workspace the user had active.
    await testPage.getByTestId("sidebar-settings-gear").click();
    await expect(testPage).toHaveURL(/\/settings/, { timeout: 10_000 });
    await expect(testPage.getByTestId("sidebar-settings-gear")).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Back out through the gear: home is the active workspace's home.
    await testPage.getByTestId("sidebar-settings-gear").click();
    await expect(testPage).toHaveURL(
      (url) =>
        url.pathname === "/office" &&
        url.searchParams.get("workspaceId") === officeSeed.workspaceId,
      { timeout: 10_000 },
    );
  });

  test("New Task follows the active workspace, not the route", async ({ testPage, seedData }) => {
    const newTask = testPage.getByTestId("create-task-button");
    const officeDialog = testPage.getByTestId("office-new-issue-dialog");
    const kanbanDialog = testPage.getByTestId("create-task-dialog");

    // Kanban workspace active: the classic Kanban create dialog, and the rich
    // Office dialog must not leak in.
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(newTask).toBeVisible({ timeout: 15_000 });
    await newTask.click();
    await expect(kanbanDialog).toBeVisible();
    await expect(officeDialog).toHaveCount(0);
    await kanbanDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(kanbanDialog).toHaveCount(0);

    // Office workspace active: the Office "New issue" dialog — and on a shared
    // route, to show the choice is the workspace's and not the URL's.
    await testPage.goto("/office");
    await expect(newTask).toBeVisible({ timeout: 15_000 });
    await testPage.goto("/stats");
    await expect(newTask).toBeVisible({ timeout: 15_000 });
    await newTask.click();
    await expect(officeDialog).toBeVisible();
    await expect(kanbanDialog).toHaveCount(0);
  });
});
