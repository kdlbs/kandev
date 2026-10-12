import { expect, test } from "../../fixtures/test-base";

test.describe("Settings composition", () => {
  test("task behavior groups are expanded within their tabs", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/task-behavior");
    await expect(testPage.getByTestId("task-behavior-group")).toHaveCount(2);
    await expect(testPage.getByTestId("task-behavior-creating-title")).toBeVisible();
    await expect(testPage.getByTestId("task-behavior-archiving-title")).toBeVisible();
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
    await expect(testPage.getByTestId("task-behavior-conversation-title")).toBeVisible();
    await testPage.getByRole("tab", { name: "Runtime", exact: true }).click();
    await expect(testPage.getByTestId("message-queue-settings")).toBeVisible();
    await expect(testPage.getByTestId("session-capacity-settings")).toBeVisible();
    await expect(testPage.getByTestId("task-behavior-settings").locator("details")).toHaveCount(0);
  });

  test("search focuses a previously visited runtime tab", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/task-behavior#setting-message-queue");
    const input = testPage.getByTestId("message-queue-max-per-session");
    await expect(input).toBeVisible();
    await testPage.getByRole("tab", { name: "Tasks", exact: true }).click();
    const search = testPage
      .getByTestId("app-sidebar-settings-mode")
      .getByTestId("settings-search")
      .getByRole("searchbox");
    await search.fill("queue");
    await testPage
      .getByTestId("settings-search-results")
      .locator('[data-settings-search-motion-key="item:task-behavior-message-queue"]')
      .click();
    await expect(input).toBeVisible();
    await expect(input).toBeFocused();
  });

  test("preferences keep notification surfaces inside shared groups", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/notifications");

    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });

  test("appearance, notifications, and editors keep one frame per group", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/appearance");
    const themeRow = testPage.getByTestId("theme-settings-card");
    await expect(themeRow).toHaveAttribute("data-settings-row", "true");
    await expect(themeRow.locator('[data-slot="card"]')).toHaveCount(0);
    await expect(
      themeRow.locator('xpath=ancestor::*[@data-settings-group-card="true"]'),
    ).toHaveCount(1);

    const changesRow = testPage.getByTestId("changes-panel-layout-card");
    const changesTrigger = changesRow.getByTestId("changes-panel-layout-select");
    const changesDescriptionId = await changesTrigger.getAttribute("aria-describedby");
    expect(changesDescriptionId).toBeTruthy();
    await expect(changesRow.locator(`[id="${changesDescriptionId}"]`)).toBeVisible();

    await testPage.goto("/settings/preferences/notifications");
    const notificationsContent = testPage.locator('[data-settings-page-content="true"]');
    await expect(notificationsContent).toBeVisible();
    await expect(notificationsContent.locator(':scope > [data-slot="card"]')).toHaveCount(0);
    await expect(notificationsContent.locator('[data-settings-group="true"]')).toHaveCount(4);

    await testPage.goto("/settings/preferences/terminal-editors");
    const editorContent = testPage.locator('[data-settings-page-content="true"]').last();
    await expect(editorContent).toBeVisible();
    await expect(editorContent.locator(':scope > [data-slot="card"]')).toHaveCount(0);
    await expect(editorContent.locator('[data-settings-group="true"]')).toHaveCount(2);
  });

  test("agent executor surfaces keep resource and create groups visible", async ({ testPage }) => {
    await testPage.goto("/settings/agents");
    await expect(testPage.getByTestId("installed-agents-actions")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();

    await testPage.goto("/settings/executors");
    await expect(testPage.locator('[data-settings-group="true"]').last()).toBeVisible();
  });

  test("workspace integration surfaces keep tabs and group chrome intact", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await expect(testPage.getByTestId("workspace-settings-shell")).toBeVisible();
    const repositoriesGroup = testPage.locator('[data-settings-group="true"]').first();
    await expect(repositoriesGroup).toBeVisible();
    await expect(
      repositoriesGroup.locator(':scope > [data-settings-group-card="true"]'),
    ).toHaveCount(0);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
    await expect(testPage.getByTestId("workspace-settings-shell")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
  });

  test("system account surfaces keep table and token groups discoverable", async ({ testPage }) => {
    await testPage.goto("/settings/system/users");
    await expect(testPage.getByTestId("users-table-card")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();

    await testPage.goto("/settings/account/tokens");
    await expect(testPage.getByTestId("api-tokens-card")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
  });
});
