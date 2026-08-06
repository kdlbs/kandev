import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type TodosLayout = {
  todosExists: boolean;
  todosGroupId: string | null;
  filesGroupId: string | null;
  rightTopOrder: string[];
};

async function createTaskWithSession(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error(`${title} did not create a session`);
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);
  return task;
}

/** Seeds a task whose session already emitted todo entries via the mock
 * agent's `e2e:plan(...)` scripting command, then settles — so the entries
 * exist only in persisted message history by the time a fresh page opens
 * the task, exactly like reopening an already-completed real task. */
async function createTaskWithTodos(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: [
        'e2e:plan([{"content":"Write tests","status":"completed"},{"content":"Implement feature","status":"in_progress"}])',
        'e2e:message("done")',
      ].join("\n"),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error(`${title} did not create a session`);
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);
  return task;
}

async function openTask(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForDockviewReady();
  return session;
}

/** Dockview tab content for the Todos panel (plain default tab, no icon). */
function todosTabContent(page: Page) {
  return page.locator(".dv-default-tab").filter({ hasText: "Todos" });
}

function todosTabWrapper(page: Page) {
  return page.locator(".dv-tab", { has: todosTabContent(page) });
}

async function readTodosLayout(page: Page): Promise<TodosLayout | null> {
  return page.evaluate(() => {
    type Panel = { id: string; group?: { id?: string; panels?: Panel[] } };
    type Api = { getPanel: (id: string) => Panel | undefined };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) return null;
    const todos = api.getPanel("todos");
    const files = api.getPanel("files");
    return {
      todosExists: !!todos,
      todosGroupId: todos?.group?.id ?? null,
      filesGroupId: files?.group?.id ?? null,
      rightTopOrder: files?.group?.panels?.map((panel) => panel.id) ?? [],
    };
  });
}

async function setTodoListPanelPreference(apiClient: ApiClient, enabled: boolean): Promise<void> {
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    show_todo_list_panel: enabled,
  });
  expect(response.ok).toBe(true);
}

test.describe("Todo list panel preference", () => {
  let baseline: boolean;

  test.beforeEach(async ({ apiClient }) => {
    const settings = await apiClient.getUserSettings();
    baseline = Boolean(settings.settings.show_todo_list_panel);
    if (baseline) await setTodoListPanelPreference(apiClient, false);
  });

  test.afterEach(async ({ apiClient }) => {
    await setTodoListPanelPreference(apiClient, baseline);
  });

  test("hides the Todos tab by default", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const task = await createTaskWithSession(apiClient, seedData, "Todos default hidden");
    await openTask(testPage, task.id);

    await expect(todosTabWrapper(testPage)).toHaveCount(0);
    await expect
      .poll(() => readTodosLayout(testPage), { timeout: 15_000 })
      .toMatchObject({ todosExists: false, rightTopOrder: ["files", "changes"] });
  });

  test("adds then removes the tab live from a second tab, without navigating away from the task or changing the selected tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createTaskWithSession(apiClient, seedData, "Todos enabled live");
    await openTask(testPage, task.id);
    await expect(todosTabWrapper(testPage)).toHaveCount(0);

    const activeRightTab = testPage
      .locator(".dv-tab.dv-active-tab")
      .filter({ hasText: /Files|Changes/ });
    await expect(activeRightTab).toHaveCount(1);
    const activeLabelBefore = await activeRightTab.textContent();

    // Toggle the preference from a second tab; `testPage` never navigates
    // away from the task — proves the change is live, not reload-dependent.
    const settingsPage = await testPage.context().newPage();
    await settingsPage.goto("/settings/general/task-actions");
    const toggle = settingsPage.getByRole("switch", { name: "Show agent todo list panel" });
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await toggle.click();
    let floatingSave = settingsPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible();

    await expect(todosTabWrapper(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(todosTabWrapper(testPage)).not.toHaveClass(/dv-active-tab/);
    await expect(activeRightTab).toHaveClass(/dv-active-tab/);
    expect(await activeRightTab.textContent()).toBe(activeLabelBefore);
    await expect
      .poll(() => readTodosLayout(testPage), { timeout: 15_000 })
      .toMatchObject({ todosExists: true, rightTopOrder: ["files", "changes", "todos"] });

    // Disable it again from the settings tab; the task tab still never navigates.
    await expect(toggle).toHaveAttribute("aria-checked", "true");
    await toggle.click();
    floatingSave = settingsPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible();
    await settingsPage.close();

    await expect(todosTabWrapper(testPage)).toHaveCount(0);
    await expect
      .poll(() => readTodosLayout(testPage), { timeout: 15_000 })
      .toMatchObject({ todosExists: false, rightTopOrder: ["files", "changes"] });
  });

  test("places the tab in a custom Default's configured group instead of the Files/Changes fallback", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await setTodoListPanelPreference(apiClient, true);

    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await expect(layouts.editor.locator(".dv-tab", { hasText: "Todos" })).toHaveCount(0);
    // Select Terminal first so the newly added Todos panel joins its group
    // (group-right-bottom) rather than the default Files/Changes fallback.
    await layouts.selectPanel("Terminal");
    await layouts.addPanel("Todos");
    await expect(layouts.editor.locator(".dv-tab", { hasText: "Todos" })).toBeVisible();
    await layouts.save();

    const saved = (await apiClient.getUserSettings()).settings.saved_layouts?.[0];
    expect(JSON.stringify(saved?.layout ?? {})).toContain("todos");

    const task = await createTaskWithSession(apiClient, seedData, "Configured Todos placement");
    await openTask(testPage, task.id);
    await expect(todosTabWrapper(testPage)).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => readTodosLayout(testPage), { timeout: 15_000 })
      .toMatchObject({
        todosExists: true,
        todosGroupId: "group-right-bottom",
        rightTopOrder: ["files", "changes"],
      });
  });

  test("shows the checklist for a session whose todos were already persisted before this page ever loaded it — not just live updates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // Todos are emitted and the session settles entirely before the
    // preference is even turned on or the task is opened, so the only way
    // the panel can show them is by deriving from persisted message
    // history (`sessionTodos.bySessionId` is populated exclusively by a
    // live WS event and is never backfilled).
    const task = await createTaskWithTodos(apiClient, seedData, "Todos from history");
    await setTodoListPanelPreference(apiClient, true);
    await openTask(testPage, task.id);

    await expect(todosTabWrapper(testPage)).toBeVisible({ timeout: 15_000 });
    await todosTabWrapper(testPage).click();
    const panel = testPage.getByTestId("todos-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("todos-panel-empty-state")).toHaveCount(0);
    await expect(panel.getByText("Write tests")).toBeVisible();
    await expect(panel.getByText("Implement feature")).toBeVisible();
    await expect(panel.getByText("1/2 completed")).toBeVisible();
  });

  test("is manually addable from the task workbench's own + menu, independent of the preference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // Preference stays off for this test (see beforeEach) — the + menu's
    // Todos row is a manual, always-available add action, not gated by the
    // preference the way the automatic visibility-sync hook is.
    const task = await createTaskWithTodos(apiClient, seedData, "Todos via plus menu");
    const session = await openTask(testPage, task.id);
    await expect(todosTabWrapper(testPage)).toHaveCount(0);

    // `addPanelButton()` targets the chat/center group's own "+" (the
    // first header action in DOM order) — deliberately not the Files/
    // Changes group where the settings-driven sync hook would default to
    // placing Todos, so this proves the manual action honors the invoking
    // group rather than always landing beside Files/Changes.
    await session.addPanelButton().click();
    const todosItem = testPage.getByRole("menuitem", { name: "Todos" });
    await expect(todosItem).toBeVisible();
    await todosItem.click();

    await expect(todosTabWrapper(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(todosTabWrapper(testPage)).toHaveClass(/dv-active-tab/);
    await expect(testPage.getByTestId("todos-panel")).toBeVisible();
    const layout = await readTodosLayout(testPage);
    expect(layout?.todosExists).toBe(true);
    expect(layout?.todosGroupId).not.toBe(layout?.filesGroupId);
  });
});
