import { test, expect } from "../../fixtures/test-base";

// @covers AC-UI-MOBILE-MENU-006.1 AC-UI-MOBILE-MENU-006.2
for (const route of ["/", "/tasks", "/threads"]) {
  test(`Home owns ${route} and Tasks keeps an independent create action`, async ({ testPage }) => {
    await testPage.goto(route);
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    await expect(menu.getByRole("link", { name: "Tasks", exact: true })).toHaveCount(0);
    await expect(menu.getByRole("link", { name: "Threads", exact: true })).toHaveCount(0);
    await expect(menu.getByRole("link", { name: "Home", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const toggle = menu.getByTestId("mobile-navigation-tasks-toggle");
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await menu.getByRole("button", { name: "New task", exact: true }).tap();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  });
}

// @covers AC-UI-MOBILE-MENU-006.3 AC-UI-MOBILE-MENU-006.4
test("empty sections retain automation and integration setup", async ({ testPage, seedData }) => {
  await testPage.goto("/stats");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await menu.getByRole("button", { name: "Automations", exact: true }).tap();
  await expect(menu.getByRole("link", { name: "Set up an automation", exact: true })).toBeVisible();
  const toggle = menu.getByTestId("mobile-navigation-tasks-toggle");
  const utilities = menu.getByText("Utilities", { exact: true });
  const headingStyle = async (element: typeof toggle) =>
    element.evaluate((el) => {
      const style = getComputedStyle(el);
      return {
        x: el.getBoundingClientRect().x,
        fontSize: style.fontSize,
        fontWeight: style.fontWeight,
      };
    });
  expect(await headingStyle(toggle)).toEqual(await headingStyle(utilities));
  for (const width of [393, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    for (const theme of ["first", "second"]) {
      await testPage.getByTestId("mobile-theme-toggle-button").tap();
      await toggle.scrollIntoViewIfNeeded();
      await testPage.screenshot({ path: test.info().outputPath(`menu-${width}-${theme}.png`) });
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
    }
  }
  const integrations = menu.getByRole("button", { name: "Integrations", exact: true });
  await expect(integrations).toHaveAttribute("aria-expanded", "false");
  await integrations.focus();
  await testPage.keyboard.press("Enter");
  await expect(integrations).toHaveAttribute("aria-expanded", "true");
  const settings = menu.getByTestId("mobile-integration-settings");
  await expect(settings).toBeVisible();
  await settings.tap();
  await expect(testPage).toHaveURL(
    new RegExp(`/settings/workspaces/${seedData.workspaceId}/integrations`),
  );
});

// @covers AC-UI-MOBILE-MENU-006.3
test("opens an automation from the shared menu", async ({ testPage, apiClient, seedData }) => {
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Daily navigation check",
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    prompt: "Review changes",
  });
  await testPage.goto("/stats");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await menu.getByRole("button", { name: "Automations", exact: true }).tap();
  await menu.getByRole("link", { name: /Daily navigation check/ }).tap();
  await expect(testPage).toHaveURL(new RegExp(`/automations/${automation.id}`));
  await expect(menu).toBeHidden();
});

// @covers AC-UI-MOBILE-MENU-007.1 AC-UI-MOBILE-MENU-007.2 AC-UI-MOBILE-MENU-007.4
for (const surface of ["home", "workbench"]) {
  test(`quick actions precede expandable content on ${surface}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task =
      surface === "workbench"
        ? await apiClient.seedTask(seedData.workspaceId, "Menu ordering", {
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
          })
        : null;
    await testPage.goto(task ? `/t/${task.task_id}` : "/");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const chat = menu.getByTestId("mobile-quick-chat-button");
    const terminal = menu.getByTestId("mobile-quick-terminal-button");
    const tasks = menu.getByTestId("mobile-navigation-tasks-toggle");
    for (const width of [393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await menu.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) =>
              Number.isFinite(animation.effect?.getComputedTiming().iterations),
            )
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      const boxes = await Promise.all(
        [menu.getByRole("link", { name: "Home", exact: true }), chat, terminal, tasks].map((item) =>
          item.boundingBox(),
        ),
      );
      const [home, quickChat, quickTerminal, heading] = boxes.map((box) => {
        expect(box).not.toBeNull();
        return box!;
      });
      expect(home.y + home.height).toBeLessThanOrEqual(quickChat.y);
      expect(quickChat.y).toBeCloseTo(quickTerminal.y);
      expect(quickChat.x + quickChat.width).toBeLessThanOrEqual(quickTerminal.x);
      expect(quickChat.y + quickChat.height).toBeLessThanOrEqual(heading.y);
      for (const box of [quickChat, quickTerminal]) {
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.width).toBeGreaterThanOrEqual(44);
      }
      for (const button of [chat, terminal]) {
        expect(await button.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
      }
    }
    await tasks.tap();
    const settings = menu.getByRole("link", { name: "Settings", exact: true });
    const stats = menu.getByRole("link", { name: "Stats", exact: true });
    expect((await settings.boundingBox())!.y).toBeLessThan((await stats.boundingBox())!.y);
    await chat.tap();
    await expect(menu).toBeHidden();
    await expect(testPage.getByRole("dialog", { name: "Quick Chat", exact: true })).toBeVisible();
  });
}

// @covers AC-UI-MOBILE-MENU-007.3
test("configured integrations collapse and retain navigation", async ({ testPage, apiClient }) => {
  await apiClient.mockGitHubSetUser("menu-demo");
  await testPage.goto("/");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const toggle = menu.getByRole("button", { name: "Integrations", exact: true });
  const github = menu.getByRole("link", { name: "GitHub", exact: true });
  await expect(github).toBeHidden();
  await toggle.tap();
  await expect(github).toBeVisible();
  await toggle.tap();
  await expect(github).toBeHidden();
  await toggle.tap();
  await github.tap();
  await expect(testPage).toHaveURL(/\/github$/);
  await expect(menu).toBeHidden();
});

// @covers AC-UI-MOBILE-MENU-007.4
test("translated quick actions fit their phone targets", async ({ testPage }) => {
  await testPage.goto("/");
  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pt-pt; path=/; SameSite=Lax";
  });
  await testPage.reload();
  await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");
  await testPage.getByTestId("app-nav-trigger").tap();
  for (const width of [393, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    for (const id of ["mobile-quick-chat-button", "mobile-quick-terminal-button"]) {
      const button = testPage.getByTestId(id);
      await expect(button).toBeVisible();
      expect(
        await button.evaluate(
          (el) => el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight,
        ),
      ).toBe(true);
      expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    await testPage.screenshot({ path: test.info().outputPath(`quick-actions-pt-${width}.png`) });
  }
});

// @covers AC-UI-MOBILE-MENU-007.2 AC-UI-MOBILE-MENU-007.4
test("collapsed sections do not leave flexible space before Utilities", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 393, height: 1200 });
  await testPage.goto("/");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await menu.getByTestId("mobile-navigation-tasks-toggle").tap();
  await expect(menu.getByRole("button", { name: "Integrations", exact: true })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  const utilities = menu.getByText("Utilities", { exact: true }).locator("..");
  for (const width of [393, 767]) {
    await testPage.setViewportSize({ width, height: 1200 });
    await expect
      .poll(() =>
        utilities.evaluate((el) => {
          const preceding = el.previousElementSibling!;
          return Math.round(
            el.getBoundingClientRect().top - preceding.getBoundingClientRect().bottom,
          );
        }),
      )
      .toBe(16);
  }
});
