import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

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

function simpleLayoutForSession(sessionId: string) {
  return {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: "group-center",
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
    await sidebar.getByRole("button", { name: /Shared environment layout child/ }).click();
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
          layout: simpleLayoutForSession(taskA.sessionId),
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

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.getByRole("menuitem", { name: "Simple", exact: true }).click();

    await expect
      .poll(() => dockviewPanelIds(testPage), {
        timeout: 10_000,
        message: "Waiting for saved layout to settle on current session",
      })
      .toContain(`session:${taskB.sessionId}`);

    const panelIds = await dockviewPanelIds(testPage);
    expect(panelIds).not.toContain(`session:${taskA.sessionId}`);
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.task.id));
    await expect(testPage.getByText("target task current")).toBeVisible();
    await expect(testPage.getByText("source task only")).not.toBeVisible();
  });
});
