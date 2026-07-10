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

test.describe("saved Dockview layouts", () => {
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
    await testPage.getByRole("menuitem", { name: /Simple/ }).click();

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
