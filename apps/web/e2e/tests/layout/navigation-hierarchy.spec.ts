import { test, expect } from "../../fixtures/test-base";
import {
  expectControlHeight,
  expectControlWidth,
  expectTouchControl,
  expectTouchSquareControl,
} from "../../helpers/control-sizing";
import { seedNavigationTaskPanel } from "../../helpers/navigation-hierarchy";
import { closeQuickTerminalTab } from "../terminal/terminal-test-helpers";
import { enableCanvasFeature } from "../canvas/canvas-fixture";

function rightEdge(box: { x: number; width: number }): number {
  return box.x + box.width;
}

function centerY(box: { y: number; height: number }): number {
  return box.y + box.height / 2;
}

// @covers AC-UI-NAV-HIERARCHY-001.1 AC-UI-NAV-HIERARCHY-001.2 AC-UI-NAV-HIERARCHY-001.3 AC-UI-NAV-HIERARCHY-001.4 AC-UI-NAV-HIERARCHY-001.5 AC-UI-NAV-HIERARCHY-001.6
test("primary action, disclosure, destination and footer have distinct behavior", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await apiClient.mockGitHubSetUser("compass-designer");
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await testPage.goto("/tasks");
  const sidebar = testPage.getByTestId("app-sidebar");
  const create = sidebar.getByTestId("create-task-button");
  const home = sidebar.getByRole("link", { name: "Home", exact: true });
  await expect(home).toHaveAttribute("aria-current", "page");
  await expectControlHeight(create, 36);
  const terminal = sidebar.getByTestId("sidebar-quick-terminal-shortcut");
  const chat = sidebar.getByTestId("sidebar-quick-chat-shortcut");
  await expect(terminal).toHaveAttribute("aria-label", "Quick terminal");
  await expect(chat).toHaveAttribute("aria-label", "Quick Chat");
  await expectControlWidth(terminal, 24);
  await expectControlWidth(chat, 24);
  await expectControlHeight(terminal, 24);
  await expectControlHeight(chat, 24);
  const terminalBox = (await terminal.boundingBox())!;
  const chatBox = (await chat.boundingBox())!;
  const createBox = (await create.boundingBox())!;
  expect(centerY(terminalBox)).toBe(centerY(createBox));
  expect(centerY(chatBox)).toBe(centerY(createBox));
  expect(rightEdge(createBox)).toBeLessThanOrEqual(terminalBox.x);
  expect(rightEdge(terminalBox)).toBeLessThanOrEqual(chatBox.x);
  const quickActions = sidebar.getByTestId("sidebar-quick-actions");
  await expect(quickActions.getByRole("button")).toHaveCount(2);
  await terminal.click();
  const terminalDialog = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
  await expect(terminalDialog.getByTestId("quick-terminal-tab")).toHaveCount(1);
  await closeQuickTerminalTab(testPage, terminalDialog.getByTestId("quick-terminal-tab").first());
  await expect(terminalDialog.getByTestId("quick-terminal-tab")).toHaveCount(0);
  await expect(terminalDialog).toBeHidden();
  await chat.click();
  const chatDialog = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
  await expect(chatDialog.getByTestId("quick-chat-setup")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(chatDialog).toBeHidden();
  await expect(chat).toBeFocused();
  const integrations = sidebar.getByRole("button", { name: "Integrations", exact: true });
  await expect(integrations).toHaveAttribute("aria-expanded", "false");
  const githubShortcut = sidebar.getByTestId("integration-header-shortcut-github");
  await expect(githubShortcut).toBeVisible();
  const integrationChevron = sidebar.getByTestId("sidebar-section-chevron-integrations");
  const shortcutBox = (await githubShortcut.boundingBox())!;
  const chevronBox = (await integrationChevron.boundingBox())!;
  const sidebarBox = (await sidebar.boundingBox())!;
  expect(shortcutBox.x).toBeGreaterThanOrEqual(sidebarBox.x);
  expect(rightEdge(shortcutBox)).toBeLessThanOrEqual(chevronBox.x);
  expect(rightEdge(shortcutBox)).toBeLessThanOrEqual(rightEdge(sidebarBox));
  await githubShortcut.click();
  await expect(testPage).toHaveURL(/\/github/);
  await expect(integrations).toHaveAttribute("aria-expanded", "false");
  await expect(githubShortcut).toHaveAttribute("aria-current", "page");
  await integrations.focus();
  await testPage.keyboard.press("Enter");
  await expect(testPage).toHaveURL(/\/github/);
  const github = sidebar
    .locator("#sidebar-section-integrations")
    .getByRole("link", { name: "GitHub", exact: true });
  await expect(github).toBeVisible();
  expect((await github.boundingBox())!.x).toBeGreaterThan((await integrations.boundingBox())!.x);
  await github.click();
  await expect(testPage).toHaveURL(/\/github/);
  await expect(github).toHaveAttribute("aria-current", "page");
  await expect(sidebar.getByTestId("sidebar-settings-gear")).toHaveText("Settings");
  const footer = sidebar.getByTestId("sidebar-footer");
  const settings = footer.getByTestId("sidebar-settings-gear");
  const stats = footer.getByTestId("sidebar-stats-button");
  const more = footer.getByTestId("sidebar-footer-more-button");
  const themeToggle = footer.getByRole("button", { name: /Switch to .* mode/i });
  const settingsBox = (await settings.boundingBox())!;
  const statsBox = (await stats.boundingBox())!;
  const themeBox = (await themeToggle.boundingBox())!;
  const moreBox = (await more.boundingBox())!;
  expect(settingsBox.y).toBe(statsBox.y);
  expect(statsBox.y).toBe(themeBox.y);
  expect(themeBox.y).toBe(moreBox.y);
  expect(rightEdge(settingsBox)).toBeLessThanOrEqual(statsBox.x);
  expect(rightEdge(statsBox)).toBeLessThanOrEqual(themeBox.x);
  expect(rightEdge(themeBox)).toBeLessThanOrEqual(moreBox.x);
  await stats.click();
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
    await sidebar.screenshot({ path: test.info().outputPath(`navigation-${theme}.png`) });
  }
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(1100, 300);
  await expect(testPage.getByTestId("app-sidebar-layout")).toHaveCSS("width", "56px");
  await expect(create).toBeVisible();
  await expect(stats).toBeVisible();
  await stats.click();
  await expect(testPage).toHaveURL(/\/stats$/);
  await create.click();
  const createDialog = testPage.getByTestId("create-task-dialog");
  await expect(createDialog).toBeVisible();
  await createDialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(createDialog).toBeHidden();
  await more.click();
  await expect(testPage.getByRole("menuitem", { name: "Stats", exact: true })).toHaveCount(0);
  await expect(
    testPage.getByRole("menuitem", { name: "Improve Kandev", exact: true }),
  ).toBeVisible();
});

test("Portuguese action labels remain contained in the compact desktop row", async ({
  testPage,
}) => {
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await testPage.goto("/tasks");
  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pt-pt; path=/; SameSite=Lax";
  });
  await testPage.reload();
  await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");

  const sidebar = testPage.getByTestId("app-sidebar");
  const create = sidebar.getByTestId("create-task-button");
  const terminal = sidebar.getByTestId("sidebar-quick-terminal-shortcut");
  const chat = sidebar.getByTestId("sidebar-quick-chat-shortcut");
  await expect(create).toHaveAccessibleName("Nova tarefa");
  await expect(terminal).toHaveAccessibleName("Terminal rápido");
  await expect(chat).toHaveAccessibleName("Chat rápido");
  const boxes = await Promise.all([create, terminal, chat].map((control) => control.boundingBox()));
  expect(boxes.every(Boolean)).toBe(true);
  expect(centerY(boxes[0]!)).toBe(centerY(boxes[1]!));
  expect(centerY(boxes[1]!)).toBe(centerY(boxes[2]!));
  expect(rightEdge(boxes[2]!)).toBeLessThanOrEqual(rightEdge((await sidebar.boundingBox())!));
  await chat.hover();
  await expect(testPage.getByRole("tooltip")).toHaveText("Chat rápido");
  await testPage.screenshot({
    path: test.info().outputPath("navigation-portuguese-long-label.png"),
  });
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
  await apiClient.mockGitHubSetUser("compass-designer");
  const releaseCanvases = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
  try {
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
      await expectTouchSquareControl(sidebar.getByTestId("sidebar-quick-chat-shortcut"));
      await expectTouchSquareControl(sidebar.getByTestId("sidebar-quick-terminal-shortcut"));
      const sidebarBox = (await sidebar.boundingBox())!;
      for (const [action, chevronId] of [
        [sidebar.getByTestId("sidebar-canvases-settings"), "sidebar-section-chevron-canvases"],
        [
          sidebar.getByTestId("integration-header-shortcut-github"),
          "sidebar-section-chevron-integrations",
        ],
      ] as const) {
        await expect(action).toBeVisible();
        await expectTouchSquareControl(action);
        const actionBox = (await action.boundingBox())!;
        const chevronBox = (await sidebar.getByTestId(chevronId).boundingBox())!;
        expect(actionBox.x).toBeGreaterThanOrEqual(sidebarBox.x);
        expect(rightEdge(actionBox)).toBeLessThanOrEqual(chevronBox.x);
        expect(rightEdge(actionBox)).toBeLessThanOrEqual(rightEdge(sidebarBox));
      }
      await expectTouchSquareControl(sidebar.getByRole("button", { name: /Switch to .* mode/i }));
      const more = sidebar.getByTestId("sidebar-footer-more-button");
      const settings = sidebar.getByTestId("sidebar-settings-gear");
      const stats = sidebar.getByTestId("sidebar-stats-button");
      const theme = sidebar.getByRole("button", { name: /Switch to .* mode/i });
      await expectTouchSquareControl(more);
      await expectTouchSquareControl(stats);
      const footerBoxes = await Promise.all(
        [settings, stats, theme, more].map((item) => item.boundingBox()),
      );
      expect(footerBoxes.every(Boolean)).toBe(true);
      expect(footerBoxes[0]!.y).toBe(footerBoxes[1]!.y);
      expect(footerBoxes[1]!.y).toBe(footerBoxes[2]!.y);
      expect(footerBoxes[2]!.y).toBe(footerBoxes[3]!.y);
      expect(rightEdge(footerBoxes[0]!)).toBeLessThanOrEqual(footerBoxes[1]!.x);
      expect(rightEdge(footerBoxes[1]!)).toBeLessThanOrEqual(footerBoxes[2]!.x);
      expect(rightEdge(footerBoxes[2]!)).toBeLessThanOrEqual(footerBoxes[3]!.x);
      await more.click();
      await expect(page.getByRole("menuitem", { name: "Stats", exact: true })).toHaveCount(0);
      await page.keyboard.press("Escape");
      await expect(more).toBeFocused();
      await stats.click();
      await expect(page).toHaveURL(/\/stats$/);
      await expectTouchSquareControl(
        sidebar
          .getByTestId("sidebar-task-item")
          .first()
          .getByRole("button", { name: "Task actions" }),
      );
    } finally {
      await context.close();
    }
  } finally {
    await releaseCanvases();
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
