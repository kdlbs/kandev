import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { getDockviewGroupWidth } from "../../helpers/dockview-resize";
import { SessionPage } from "../../pages/session-page";

async function createTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

type DesktopTaskOptions = {
  viewport: { width: number; height: number };
  layout?: string;
};

async function openDesktopTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  options: DesktopTaskOptions,
) {
  await page.setViewportSize(options.viewport);
  const task = await createTask(apiClient, seedData, title);
  if (!task.session_id) throw new Error(`${title} did not return a session_id`);
  await page.goto(`/t/${task.id}${options.layout ? `?layout=${options.layout}` : ""}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForDockviewReady();
  return { session, sessionId: task.session_id };
}

test.describe("right-panel visibility", () => {
  test("desktop hides and restores the right column while reclaiming center width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, sessionId } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Desktop right-panel visibility",
      { viewport: { width: 1600, height: 900 } },
    );
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();
    await expect(session.terminal).toBeVisible();

    const centerWidthBefore = await testPage.evaluate((id) => {
      type Api = {
        getPanel: (panelId: string) => { group: { width: number } } | undefined;
      };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      return api?.getPanel(`session:${id}`)?.group.width ?? 0;
    }, sessionId);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(session.files).toHaveCount(0);
    await expect(session.terminal).toHaveCount(0);
    await expect(session.activeChat()).toBeVisible();

    await expect
      .poll(
        () =>
          testPage.evaluate((id) => {
            type Api = {
              getPanel: (panelId: string) => { group: { width: number } } | undefined;
            };
            const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
            return api?.getPanel(`session:${id}`)?.group.width ?? 0;
          }, sessionId),
        { message: "center group did not reclaim the right-column width" },
      )
      .toBeGreaterThan(centerWidthBefore + 100);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(session.files).toHaveCount(0);
    await expect(session.terminal).toHaveCount(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();
    await expect(session.terminal).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();
    await expect(session.terminal).toBeVisible();
    await session.expectLayoutHealthy();
  });

  test("compact desktop disables the action when it has one workbench region", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Compact desktop right-panel visibility",
      { viewport: { width: 900, height: 800 } },
    );
    await expect(testPage.getByTestId("tablet-task-layout")).toHaveCount(0);
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toBeDisabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toHaveAttribute("title", "No separate right pane to hide");
    await expect(toggle.locator("..")).toHaveAttribute(
      "aria-label",
      "No separate right pane to hide",
    );

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeDisabled();
    await expect(session.activeChat()).toBeVisible();
  });

  test("Plan and Preview layouts toggle their contextual right pane", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Plan contextual right pane",
      { viewport: { width: 1600, height: 900 }, layout: "plan" },
    );
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    const plan = testPage.getByTestId("plan-panel");
    await expect(toggle).toBeEnabled();
    await expect(plan).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(plan).toHaveCount(0);
    await expect(session.activeChat()).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(plan).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(plan).toBeVisible();
  });

  test("Preview layout toggles Browser instead of a standard right stack", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Preview contextual right pane",
      { viewport: { width: 1600, height: 900 } },
    );
    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.locator('[data-testid="layout-preset-item"][data-preset-id="preview"]').click();
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    const browser = testPage.getByTestId("browser-panel");
    await expect(toggle).toBeEnabled();
    await expect(browser).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(browser).toHaveCount(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(browser).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(browser).toBeVisible();
  });

  test("keeps the toggle disabled while maximized and restores visibility after exit and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Maximized right-panel visibility",
      { viewport: { width: 1600, height: 900 } },
    );
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");

    await session.clickTab("Files");
    await session.clickMaximize();
    await session.expectMaximized();

    await expect(toggle).toBeDisabled();
    await expect(toggle).toHaveAttribute(
      "title",
      "Right pane is unavailable while a panel is maximized",
    );
    await expect(toggle.locator("..")).toHaveAttribute(
      "aria-label",
      "Right pane is unavailable while a panel is maximized",
    );

    await session.clickMaximize();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();
    await session.expectLayoutHealthy();
  });

  test("supports keyboard activation while keeping focus on the persistent control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openDesktopTask(testPage, apiClient, seedData, "Keyboard right-panel visibility", {
      viewport: { width: 1600, height: 900 },
    });
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeEnabled();

    await toggle.focus();
    await expect(toggle).toBeFocused();
    await toggle.press("Enter");
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeFocused();

    await toggle.press("Space");
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(toggle).toBeFocused();
  });

  test("tablet hides Files and Terminal, preserves Chat, and restores the session choice", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const task = await createTask(apiClient, seedData, "Tablet right-panel visibility");
    if (!task.session_id) throw new Error("tablet task did not return a session_id");
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const environmentId = sessions.find(
      (session) => session.id === task.session_id,
    )?.task_environment_id;
    if (!environmentId) throw new Error("tablet task is missing an environment id");
    const terminal = await apiClient.wsRequest<{ terminal_id: string }>("user_shell.create", {
      task_id: task.id,
      task_environment_id: environmentId,
    });
    await tabletTestPage.goto(`/t/${task.id}`);
    const session = new SessionPage(tabletTestPage);
    const tabletFiles = tabletTestPage.getByTestId("file-tree-scroll");
    const tabletTerminalTab = tabletTestPage.getByTestId(`terminal-tab-${terminal.terminal_id}`);
    await session.waitForLoad();
    await expect(tabletTestPage.getByTestId("tablet-task-layout")).toBeVisible();

    const toggle = tabletTestPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(tabletFiles).toBeVisible();
    await expect(tabletTerminalTab).toBeVisible();

    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(tabletFiles).toHaveCount(0);
    await expect(tabletTerminalTab).toHaveCount(0);
    await expect(session.activeChat()).toBeVisible();
    await assertNoDocumentHorizontalOverflow(tabletTestPage, "tablet right-panel visibility");

    const stored = await tabletTestPage.evaluate((sessionId) => {
      const raw = window.localStorage.getItem("layout-columns-by-session");
      const layouts = raw ? (JSON.parse(raw) as Record<string, { right?: boolean }>) : {};
      return layouts[sessionId]?.right;
    }, task.session_id);
    expect(stored).toBe(false);

    await tabletTestPage.reload();
    await session.waitForLoad();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(tabletFiles).toHaveCount(0);

    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(tabletFiles).toBeVisible();
    await expect(tabletTerminalTab).toBeVisible();
  });
});

for (const layout of ["default", "plan", "preview"] as const) {
  test(`restores manually resized ${layout} pane width after toggling`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      `Resized ${layout} pane`,
      { viewport: { width: 1600, height: 900 } },
    );
    if (layout !== "default") {
      await testPage.getByTestId("layout-preset-trigger").click();
      await testPage
        .locator(`[data-testid="layout-preset-item"][data-preset-id="${layout}"]`)
        .click();
      await expect(
        testPage.getByTestId(layout === "plan" ? "plan-panel" : "browser-panel"),
      ).toBeVisible();
    }
    const panelId = { default: "files", preview: "browser", plan: "plan" }[layout]!;
    const before = await getDockviewGroupWidth(testPage, panelId);
    const sashBox = await testPage.evaluate(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const api = (window as any).__dockviewApi__;
      const sashes = api.component.gridview.root.splitview.sashes;
      const rect = sashes.at(-1).container.getBoundingClientRect();
      return { x: rect.x + rect.width / 2, y: rect.y + 100 };
    });
    await testPage.mouse.move(sashBox.x, sashBox.y);
    await testPage.mouse.down();
    await testPage.mouse.move(sashBox.x - 100, sashBox.y, { steps: 10 });
    await testPage.mouse.up();
    await expect.poll(() => getDockviewGroupWidth(testPage, panelId)).toBeGreaterThan(before + 50);
    const resized = await getDockviewGroupWidth(testPage, panelId);
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    for (let cycle = 0; cycle < 3; cycle++) {
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-expanded", "false");
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-expanded", "true");
      await session.waitForDockviewReady();
      await expect
        .poll(async () => Math.abs((await getDockviewGroupWidth(testPage, panelId)) - resized))
        .toBeLessThanOrEqual(2);
    }
  });
}
