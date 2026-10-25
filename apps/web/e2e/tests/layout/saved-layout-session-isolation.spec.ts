import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const CREATE_PLAN_SCRIPT = [
  'e2e:thinking("creating plan")',
  "e2e:delay(100)",
  'e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"## Focus regression\\n\\nKeep Agent selected","title":"Focus plan"})',
  "e2e:delay(100)",
  'e2e:message("focus regression agent")',
].join("\n");

async function createFinishedTaskWithSession(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error(`${title} did not return a session_id`);

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000, message: `Waiting for ${title} session to finish` },
    )
    .toBe(true);

  return { task, sessionId: task.session_id };
}

function simpleLayoutForSession(sessionId: string, groupId = "group-center") {
  return {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: groupId,
            panels: [
              {
                id: `session:${sessionId}`,
                component: "chat",
                title: "Agent",
                tabComponent: "sessionTab",
                params: { sessionId },
              },
            ],
            activePanel: `session:${sessionId}`,
          },
        ],
      },
    ],
  };
}

async function dockviewPanelIds(page: Page) {
  return page.evaluate(() => {
    type DockviewWindow = Window & {
      __dockviewApi__?: { panels: Array<{ id: string }> };
    };
    return ((window as DockviewWindow).__dockviewApi__?.panels ?? []).map((p) => p.id);
  });
}

async function dockviewDefaultTree(page: Page, sessionId: string) {
  return page.evaluate((currentSessionId) => {
    type GridNode =
      | { type: "branch"; data: GridNode[] }
      | { type: "leaf"; data: { views: string[] } };
    type Api = {
      getPanel: (id: string) => { group: { id: string } } | null;
      toJSON: () => { grid: { orientation: string; root: GridNode } };
    };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) throw new Error("dockview api not exposed");
    const layout = api.toJSON();
    const root = layout.grid.root;
    const rootColumns = root.type === "branch" ? root.data : [];
    const rightColumnPreserved = rootColumns.some(
      (column) =>
        column.type === "branch" &&
        column.data.some(
          (group) =>
            group.type === "leaf" &&
            (group.data.views.includes("files") || group.data.views.includes("changes")),
        ) &&
        column.data.some(
          (group) => group.type === "leaf" && group.data.views.includes("terminal-default"),
        ),
    );

    return {
      orientation: layout.grid.orientation,
      centerGroupId: api.getPanel(`session:${currentSessionId}`)?.group.id ?? null,
      rootColumnCount: rootColumns.length,
      rightColumnPreserved,
    };
  }, sessionId);
}

function focusSplitLayout() {
  return {
    grid: {
      root: {
        type: "branch",
        size: 700,
        data: [
          {
            type: "leaf",
            size: 900,
            data: {
              id: "group-center",
              views: ["chat", "plan"],
              activeView: "chat",
            },
          },
          {
            type: "branch",
            size: 360,
            data: [
              {
                type: "leaf",
                size: 420,
                data: {
                  id: "group-right-top",
                  views: ["files", "changes"],
                  activeView: "files",
                },
              },
              {
                type: "leaf",
                size: 280,
                data: {
                  id: "group-right-bottom",
                  views: ["terminal-default"],
                  activeView: "terminal-default",
                },
              },
            ],
          },
        ],
      },
      height: 700,
      width: 1260,
      orientation: "HORIZONTAL",
    },
    panels: {
      chat: { id: "chat", contentComponent: "chat", title: "Agent" },
      plan: {
        id: "plan",
        contentComponent: "plan",
        tabComponent: "planTab",
        title: "Plan",
      },
      files: { id: "files", contentComponent: "files", title: "Files" },
      changes: {
        id: "changes",
        contentComponent: "changes",
        tabComponent: "changesTab",
        title: "Changes",
      },
      "terminal-default": {
        id: "terminal-default",
        contentComponent: "terminal",
        tabComponent: "terminalTab",
        title: "Terminal",
      },
    },
    activeGroup: "group-right-top",
  };
}

async function triggerFocusSplitReconciliation(page: Page, taskId: string, sessionId: string) {
  await page.evaluate(
    ({ currentTaskId, currentSessionId }) => {
      type Panel = {
        id: string;
        group: { id: string };
        api: { close: () => void; setActive: () => void };
      };
      type StoreState = {
        taskSessionsByTask: { itemsByTaskId: Record<string, unknown[]> };
      };
      type TestWindow = Window & {
        __KANDEV_E2E_STORE__?: {
          getState: () => StoreState;
          setState: (updater: (state: StoreState) => void) => void;
        };
        __dockviewApi__?: {
          addPanel: (options: object) => Panel;
          getPanel: (id: string) => Panel | null;
        };
        __focusTestSessions__?: unknown[];
      };
      const win = window as TestWindow;
      const api = win.__dockviewApi__;
      const store = win.__KANDEV_E2E_STORE__;
      if (!api || !store) throw new Error("Dockview or E2E store bridge missing");

      const sessionPanel = api.getPanel(`session:${currentSessionId}`);
      const centerGroupId = sessionPanel?.group.id ?? api.getPanel("plan")?.group.id;
      if (!centerGroupId) throw new Error("Center group missing");
      const chatPanel =
        api.getPanel("chat") ??
        api.addPanel({
          id: "chat",
          component: "chat",
          title: "Agent",
          position: { referenceGroup: centerGroupId, index: 0 },
          inactive: true,
        });
      const filesPanel = api.getPanel("files");
      if (!filesPanel) throw new Error("Files panel missing");

      chatPanel.api.setActive();
      filesPanel.api.setActive();
      sessionPanel?.api.close();

      const sessions = store.getState().taskSessionsByTask.itemsByTaskId[currentTaskId];
      if (!sessions?.length) throw new Error("Task session list missing");
      win.__focusTestSessions__ = sessions;
      store.setState((state) => {
        state.taskSessionsByTask.itemsByTaskId[currentTaskId] = [];
      });
    },
    { currentTaskId: taskId, currentSessionId: sessionId },
  );

  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );

  await page.evaluate((currentTaskId) => {
    type StoreState = {
      taskSessionsByTask: { itemsByTaskId: Record<string, unknown[]> };
    };
    type TestWindow = Window & {
      __KANDEV_E2E_STORE__?: {
        setState: (updater: (state: StoreState) => void) => void;
      };
      __focusTestSessions__?: unknown[];
    };
    const win = window as TestWindow;
    const store = win.__KANDEV_E2E_STORE__;
    const sessions = win.__focusTestSessions__;
    if (!store || !sessions) throw new Error("Focus test session snapshot missing");
    store.setState((state) => {
      state.taskSessionsByTask.itemsByTaskId[currentTaskId] = sessions;
    });
  }, taskId);
}

test.describe("saved Dockview layouts", () => {
  test("keeps the desktop split tree when switching tasks in one environment", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await createFinishedTaskWithSession(
      apiClient,
      seedData,
      "Shared environment layout parent",
      '/e2e:message("parent session")',
    );
    const child = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Shared environment layout child",
      seedData.agentProfileId,
      {
        description: '/e2e:message("child session")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        parent_id: parent.task.id,
        workspace_mode: "inherit_parent",
      },
    );
    if (!child.session_id) throw new Error("child task did not return a session_id");

    await expect
      .poll(async () => {
        const [{ sessions: parentSessions }, { sessions: childSessions }] = await Promise.all([
          apiClient.listTaskSessions(parent.task.id),
          apiClient.listTaskSessions(child.id),
        ]);
        const parentEnvironmentId = parentSessions[0]?.task_environment_id;
        const childEnvironmentId = childSessions[0]?.task_environment_id;
        return parentEnvironmentId && childEnvironmentId === parentEnvironmentId;
      })
      .toBe(true);

    await testPage.goto(`/t/${parent.task.id}`);
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("parent session")).toBeVisible({ timeout: 30_000 });

    const sidebar = testPage.getByTestId("app-sidebar");
    // Not getByRole("button", {name}): the row's title now also renders its
    // own nested button (the keyboard-operable title-preview trigger) with
    // the same accessible name, so a name-based query is ambiguous.
    await sidebar.locator(`[data-task-row-id="${child.id}"]`).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${child.id}(?:\\?|$)`), { timeout: 10_000 });

    await expect
      .poll(() => dockviewPanelIds(testPage), {
        timeout: 10_000,
        message: "Waiting for the child session panel after the same-environment switch",
      })
      .toContain(`session:${child.session_id}`);
    expect(await dockviewPanelIds(testPage)).not.toContain(`session:${parent.sessionId}`);
    await expect(testPage.getByTestId(`session-tab-${child.session_id}`)).toBeVisible();
    await expect(testPage.getByTestId("watermark-add-panel-btn")).toHaveCount(0);

    await expect
      .poll(() => dockviewDefaultTree(testPage, child.session_id!))
      .toEqual({
        orientation: "HORIZONTAL",
        centerGroupId: "group-center",
        rootColumnCount: 2,
        rightColumnPreserved: true,
      });

    await testPage.reload();
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => dockviewDefaultTree(testPage, child.session_id!))
      .toEqual({
        orientation: "HORIZONTAL",
        centerGroupId: "group-center",
        rootColumnCount: 2,
        rightColumnPreserved: true,
      });
    await expect(testPage.getByTestId(`session-tab-${child.session_id}`)).toBeVisible();
    await expect(testPage.getByTestId("watermark-add-panel-btn")).toHaveCount(0);
  });

  test("saved chat-only layouts keep the current task session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskA = await createFinishedTaskWithSession(
      apiClient,
      seedData,
      "Saved Layout Source Task",
      '/e2e:message("source task only")',
    );
    const taskB = await createFinishedTaskWithSession(
      apiClient,
      seedData,
      "Saved Layout Target Task",
      '/e2e:message("target task current")',
    );

    const savedLayoutGroupId = "group-saved-simple";
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      saved_layouts: [
        {
          id: "layout-default-stale-session",
          name: "Default Chat",
          is_default: true,
          layout: simpleLayoutForSession(taskA.sessionId),
          created_at: new Date().toISOString(),
        },
        {
          id: "layout-simple-stale-session",
          name: "Simple",
          is_default: false,
          layout: simpleLayoutForSession(taskA.sessionId, savedLayoutGroupId),
          created_at: new Date().toISOString(),
        },
      ],
    });

    await testPage.goto(`/t/${taskB.task.id}`);
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("target task current")).toBeVisible({ timeout: 30_000 });

    await expect
      .poll(() => dockviewPanelIds(testPage), {
        timeout: 10_000,
        message: "Waiting for default layout to settle on current session",
      })
      .toContain(`session:${taskB.sessionId}`);
    expect(await dockviewPanelIds(testPage)).not.toContain(`session:${taskA.sessionId}`);

    const presetTrigger = testPage.getByTestId("layout-preset-trigger");
    if ((await presetTrigger.getAttribute("aria-expanded")) !== "true") {
      await presetTrigger.click();
    }
    await testPage.getByRole("menuitem", { name: "Simple", exact: true }).click();

    await expect
      .poll(async () => (await dockviewDefaultTree(testPage, taskB.sessionId)).centerGroupId, {
        timeout: 10_000,
        message: "Waiting for saved layout to finish applying",
      })
      .toBe(savedLayoutGroupId);
    await expect(testPage.getByRole("menu")).toHaveCount(0);

    const panelIds = await dockviewPanelIds(testPage);
    expect(panelIds).not.toContain(`session:${taskA.sessionId}`);
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.task.id));
    await expect(testPage.getByText("target task current")).toBeVisible();
    await expect(testPage.getByText("source task only")).not.toBeVisible();

    if ((await presetTrigger.getAttribute("aria-expanded")) !== "true") {
      await presetTrigger.click();
    }
    await testPage
      .locator('[data-testid="layout-saved-delete"][data-layout-id="layout-simple-stale-session"]')
      .click();
    const confirmation = testPage.getByTestId("layout-saved-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toHaveLength(2);

    if ((await presetTrigger.getAttribute("aria-expanded")) !== "true") {
      await presetTrigger.click();
    }
    await testPage
      .locator('[data-testid="layout-saved-delete"][data-layout-id="layout-simple-stale-session"]')
      .click();
    const secondConfirmation = testPage.getByTestId("layout-saved-delete-confirm-popover");
    await expect(secondConfirmation).toBeVisible();
    await testPage
      .locator('[data-testid="layout-saved-delete-confirm-popover"]')
      .getByTestId("layout-saved-delete-confirm")
      .click();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.saved_layouts)
      .toHaveLength(1);
    expect(await dockviewPanelIds(testPage)).toContain(`session:${taskB.sessionId}`);
    expect(await dockviewPanelIds(testPage)).not.toContain(`session:${taskA.sessionId}`);
  });

  test("preserves Agent selection while Files owns global focus", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { task, sessionId } = await createFinishedTaskWithSession(
      apiClient,
      seedData,
      "Dockview focus preservation",
      '/e2e:message("focus regression agent")',
    );
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const environmentId = sessions[0]?.task_environment_id;
    if (!environmentId) throw new Error("Task environment id missing");

    await testPage.addInitScript(
      ({ envId, layout }) => {
        window.sessionStorage.setItem(
          `kandev.dockview.env-layout-v3.${envId}`,
          JSON.stringify(layout),
        );
      },
      { envId: environmentId, layout: focusSplitLayout() },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId(`session-tab-${sessionId}`)).toBeVisible({
      timeout: 15_000,
    });
    await session.clickSessionChatTab();
    await session.waitForLoad();
    await session.sendMessage(CREATE_PLAN_SCRIPT);
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });
    await expect.poll(async () => (await apiClient.getTaskPlan(task.id))?.created_by).toBe("agent");

    const planTab = testPage.locator(".dv-tab", {
      has: testPage.locator(".dv-default-tab:has-text('Plan')"),
    });
    await expect(planTab).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("plan-tab-indicator")).toBeVisible({ timeout: 15_000 });

    await triggerFocusSplitReconciliation(testPage, task.id, sessionId);

    const sessionTab = testPage.locator(".dv-tab", {
      has: testPage.getByTestId(`session-tab-${sessionId}`),
    });
    await expect(sessionTab).toHaveClass(/dv-active-tab/);
    await expect(planTab).not.toHaveClass(/dv-active-tab/);
    await expect(testPage.getByTestId("plan-tab-indicator")).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(
          () =>
            (
              window as unknown as {
                __dockviewApi__?: { activePanel?: { id: string }; panels: Array<{ id: string }> };
              }
            ).__dockviewApi__?.activePanel?.id ?? null,
        ),
      )
      .toBe("files");
    expect(
      (await dockviewPanelIds(testPage)).filter((id) => id === `session:${sessionId}`),
    ).toHaveLength(1);
  });
});
