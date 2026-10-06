import { test, expect } from "../../fixtures/test-base";
import {
  expectControlHeight,
  expectTouchControl,
  expectTouchSquareControl,
} from "../../helpers/control-sizing";
import { seedNavigationTaskPanel } from "../../helpers/navigation-hierarchy";

// @covers AC-UI-NAV-HIERARCHY-001.1 AC-UI-NAV-HIERARCHY-001.2 AC-UI-NAV-HIERARCHY-001.3 AC-UI-NAV-HIERARCHY-001.4 AC-UI-NAV-HIERARCHY-001.5 AC-UI-NAV-HIERARCHY-001.6
test("primary action, disclosure, destination and footer have distinct behavior", async ({
  testPage,
  apiClient,
}) => {
  await apiClient.mockGitHubSetUser("compass-designer");
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await testPage.goto("/tasks");
  const sidebar = testPage.getByTestId("app-sidebar");
  const create = sidebar.getByTestId("create-task-button");
  const home = sidebar.getByRole("link", { name: "Home", exact: true });
  await expect(home).toHaveAttribute("aria-current", "page");
  await expect(create).toHaveAttribute("data-variant", "outline");
  await expectControlHeight(create, 44);
  const chat = sidebar.getByTestId("sidebar-quick-chat-shortcut");
  const terminal = sidebar.getByTestId("sidebar-quick-terminal-shortcut");
  await expect(chat).toHaveText("Quick Chat");
  await expect(terminal).toHaveText("Terminal");
  await expectControlHeight(chat, 28);
  await expectControlHeight(terminal, 28);
  const chatBox = (await chat.boundingBox())!;
  const terminalBox = (await terminal.boundingBox())!;
  const createBox = (await create.boundingBox())!;
  const utilities = sidebar.getByRole("group", { name: "Utilities", exact: true });
  await expect(utilities).toHaveCSS("transform", "none");
  const utilitiesBox = (await utilities.boundingBox())!;
  expect(utilitiesBox.x).toBe(createBox.x);
  expect(utilitiesBox.width).toBe(createBox.width);
  expect(Math.abs(chatBox.width - terminalBox.width)).toBeLessThan(1);
  expect(chatBox.width + terminalBox.width).toBeGreaterThan(createBox.width * 0.9);
  for (const utility of [chat, terminal]) {
    await expect(utility).toHaveAttribute("data-variant", "ghost");
    await expect(utility).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(utility).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");
  }
  expect(chatBox.y).toBe(terminalBox.y);
  expect(chatBox.y).toBeGreaterThan((await create.boundingBox())!.y);
  expect((await create.boundingBox())!.y).toBeLessThan((await home.boundingBox())!.y);
  const integrations = sidebar.getByRole("button", { name: "Integrations", exact: true });
  await expect(integrations).toHaveAttribute("aria-expanded", "false");
  await expect(sidebar.getByRole("link", { name: "GitHub", exact: true })).toBeHidden();
  await integrations.focus();
  await testPage.keyboard.press("Enter");
  await expect(testPage).toHaveURL(/\/tasks$/);
  const github = sidebar.getByRole("link", { name: "GitHub", exact: true });
  await expect(github).toBeVisible();
  expect((await github.boundingBox())!.x).toBeGreaterThan((await integrations.boundingBox())!.x);
  await github.click();
  await expect(testPage).toHaveURL(/\/github/);
  await expect(github).toHaveAttribute("aria-current", "page");
  await expect(sidebar.getByTestId("sidebar-settings-gear")).toHaveText("Settings");
  const footer = sidebar.getByTestId("sidebar-footer");
  const settings = footer.getByTestId("sidebar-settings-gear");
  const more = footer.getByRole("button", { name: "Show more actions", exact: true });
  const themeToggle = footer.getByRole("button", { name: /Switch to .* mode/i });
  expect((await settings.boundingBox())!.y).toBe((await more.boundingBox())!.y);
  expect((await themeToggle.boundingBox())!.y).toBe((await more.boundingBox())!.y);
  await expect(sidebar.getByTestId("sidebar-stats-button")).toBeHidden();
  await more.click();
  await expect(testPage.getByRole("menuitem", { name: "Stats", exact: true })).toBeVisible();
  await expect(
    testPage.getByRole("menuitem", { name: "Improve Kandev", exact: true }),
  ).toBeVisible();
  await testPage.getByRole("menuitem", { name: "Stats", exact: true }).click();
  await expect(testPage).toHaveURL(/\/stats$/);
  await expect(testPage.getByTestId("sidebar-footer-menu")).toBeHidden();
  for (const theme of ["dark", "light"] as const) {
    const toggle = sidebar.getByRole("button", { name: /Switch to .* mode/i });
    if (
      (await testPage.locator("html").getAttribute("class"))?.includes("dark") !==
      (theme === "dark")
    )
      await toggle.click();
    await expect(testPage.locator("html")).toHaveClass(new RegExp(`\\b${theme}\\b`));
    await testPage.screenshot({ path: test.info().outputPath(`navigation-${theme}.png`) });
  }
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(1100, 300);
  await expect(testPage.getByTestId("app-sidebar-layout")).toHaveCSS("width", "56px");
  await expect(create).toBeVisible();
  await create.click();
  await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
});

test("an unconfigured workspace retains integration setup", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await testPage.goto("/tasks");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Integrations", exact: true }).click();
  await sidebar.getByRole("link", { name: "Integration settings", exact: true }).click();
  await expect(testPage).toHaveURL(
    new RegExp(`/settings/workspaces/${seedData.workspaceId}/integrations`),
  );
});

// @covers AC-UI-NAV-HIERARCHY-001.5
test("release notes stay reachable and readable after being seen with notifications disabled", async ({
  testPage,
  apiClient,
}) => {
  await apiClient.saveUserSettings({
    show_release_notification: false,
    release_notes_last_seen_version: "999.0.0",
  });
  await testPage.setViewportSize({ width: 768, height: 900 });
  await testPage.goto("/tasks");
  const more = testPage.getByTestId("sidebar-footer-more-button");
  for (let attempt = 0; attempt < 2; attempt++) {
    await more.click();
    const notes = testPage.getByTestId("sidebar-release-notes-button");
    await expect(notes).toBeVisible();
    expect(
      await notes.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
        );
      }),
    ).toBe(true);
    await notes.click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(".markdown-body").first()).toContainText(/\S.{20}/);
    await testPage.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  }
});

test("tablet coarse-pointer navigation has touch targets", async ({
  browser,
  backend,
  apiClient,
  seedData,
}) => {
  await seedNavigationTaskPanel(apiClient, seedData);
  const context = await browser.newContext({
    viewport: { width: 768, height: 1024 },
    hasTouch: true,
  });
  const page = await context.newPage();
  try {
    await page.goto(`${backend.baseUrl}/tasks`);
    const sidebar = page.getByTestId("app-sidebar");
    await expect(sidebar).toBeVisible();
    await expectTouchControl(sidebar.getByTestId("create-task-button"));
    await expectTouchControl(sidebar.getByRole("button", { name: "Integrations", exact: true }));
    await expectTouchControl(sidebar.getByTestId("sidebar-settings-gear"));
    await expectTouchControl(sidebar.getByTestId("sidebar-quick-chat-shortcut"));
    await expectTouchControl(sidebar.getByTestId("sidebar-quick-terminal-shortcut"));
    await expectTouchSquareControl(sidebar.getByRole("button", { name: /Switch to .* mode/i }));
    const more = sidebar.getByTestId("sidebar-footer-more-button");
    await expectTouchSquareControl(more);
    await more.click();
    const stats = page.getByRole("menuitem", { name: "Stats", exact: true });
    await expect
      .poll(async () => (await stats.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await page.keyboard.press("Escape");
    await expect(more).toBeFocused();
    await expectTouchSquareControl(
      sidebar
        .getByTestId("sidebar-task-item")
        .first()
        .getByRole("button", { name: "Task actions" }),
    );
  } finally {
    await context.close();
  }
});

test("a narrow fine-pointer window retains the complete phone menu", async ({
  browser,
  backend,
  apiClient,
  seedData,
}) => {
  await seedNavigationTaskPanel(apiClient, seedData);
  const context = await browser.newContext({
    viewport: { width: 767, height: 900 },
    hasTouch: false,
  });
  const page = await context.newPage();
  try {
    await page.goto(`${backend.baseUrl}/tasks`);
    await page.getByTestId("app-nav-trigger").click();
    const menu = page.getByTestId("app-nav-sheet");
    await expectTouchControl(menu.getByTestId("mobile-new-task-button"));
    await expectTouchControl(menu.getByTestId("sidebar-group-header").first());
    const action = menu
      .getByTestId("sidebar-task-item")
      .first()
      .getByRole("button", { name: "Task actions" });
    await expectTouchSquareControl(action);
    await action.click();
    await expect(page.getByTestId("task-context-priority")).toBeVisible();
  } finally {
    await context.close();
  }
});
