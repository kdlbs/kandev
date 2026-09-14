import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

async function createTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function openDesktopTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  viewport: { width: number; height: number },
) {
  await page.setViewportSize(viewport);
  const task = await createTask(apiClient, seedData, title);
  if (!task.session_id) throw new Error(`${title} did not return a session_id`);
  await page.goto(`/t/${task.id}`);
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
      { width: 1600, height: 900 },
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

  test("compact desktop can explicitly reopen its initially hidden right column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Compact desktop right-panel visibility",
      { width: 900, height: 800 },
    );
    await expect(testPage.getByTestId("tablet-task-layout")).toHaveCount(0);
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(session.files).toBeVisible();
    await expect(session.terminal).toBeVisible();
  });

  test("supports keyboard activation while keeping focus on the persistent control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openDesktopTask(testPage, apiClient, seedData, "Keyboard right-panel visibility", {
      width: 1600,
      height: 900,
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
