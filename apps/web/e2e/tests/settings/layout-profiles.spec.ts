import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type DockviewSnapshot = {
  panelIds: string[];
  filesGroupId: string | null;
  changesGroupId: string | null;
  rightGroupOrder: string[];
};

function noTerminalLayout() {
  return {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: "group-center",
            panels: [
              {
                id: "chat",
                component: "chat",
                title: "Agent",
                tabComponent: "permanentTab",
              },
            ],
            activePanel: "chat",
          },
        ],
      },
      {
        id: "right",
        pinned: true,
        width: 350,
        groups: [
          {
            id: "group-right-top",
            panels: [
              { id: "files", component: "files", title: "Files" },
              {
                id: "changes",
                component: "changes",
                title: "Changes",
                tabComponent: "changesTab",
              },
            ],
            activePanel: "files",
          },
        ],
      },
    ],
  };
}

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
  if (!task.session_id) throw new Error(`${title} did not return a session_id`);
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

async function dockviewSnapshot(page: Page): Promise<DockviewSnapshot> {
  return page.evaluate(() => {
    type Panel = { id: string; group?: { id: string; panels: Panel[] } };
    type Api = { panels: Panel[]; getPanel: (id: string) => Panel | undefined };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) throw new Error("dockview api not exposed");
    const files = api.getPanel("files");
    const changes = api.getPanel("changes");
    return {
      panelIds: api.panels.map((panel) => panel.id),
      filesGroupId: files?.group?.id ?? null,
      changesGroupId: changes?.group?.id ?? null,
      rightGroupOrder: files?.group?.panels.map((panel) => panel.id) ?? [],
    };
  });
}

async function expectNoTerminalDefault(page: Page): Promise<void> {
  await expect
    .poll(() => dockviewSnapshot(page), {
      timeout: 10_000,
      message: "Waiting for the no-terminal default layout",
    })
    .toMatchObject({
      filesGroupId: "group-right-top",
      changesGroupId: "group-right-top",
      rightGroupOrder: ["files", "changes"],
    });
  const snapshot = await dockviewSnapshot(page);
  expect(snapshot.panelIds).not.toContain("terminal-default");
}

async function ordinaryShells(apiClient: ApiClient, taskId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const environmentId = sessions[0]?.task_environment_id;
  if (!environmentId) throw new Error(`Task ${taskId} has no environment`);
  const response = await apiClient.wsRequest<{ shells?: Array<{ kind?: string }> }>(
    "user_shell.list",
    {
      task_id: taskId,
      task_environment_id: environmentId,
      include_parked: true,
    },
  );
  return (response.shells ?? []).filter((shell) => shell.kind === "ordinary");
}

test.describe("Task layout profile defaults", () => {
  test("edits and saves the built-in Default without duplicating it first", async ({
    testPage,
    apiClient,
  }) => {
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.removePanel("Terminal");
    await layouts.renameSelected("Focused default");
    await layouts.save();

    const response = await apiClient.getUserSettings();
    const saved = response.settings.saved_layouts;
    expect(saved).toHaveLength(1);
    expect(saved[0]).toMatchObject({ name: "Focused default", is_default: true });
    expect(JSON.stringify(saved[0].layout)).not.toContain("terminal-default");
  });

  test("fresh tasks use the no-terminal default while existing tasks wait for Reset Layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskA = await createTaskWithSession(apiClient, seedData, "Existing Layout Task");
    await openTask(testPage, taskA.id);

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.locator('[data-testid="layout-preset-item"][data-preset-id="default"]').click();
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);

    await apiClient.saveUserSettings({
      saved_layouts: [
        {
          id: "focused-default",
          name: "Focused default",
          is_default: true,
          layout: noTerminalLayout(),
          created_at: new Date().toISOString(),
        },
      ],
    });

    const taskB = await createTaskWithSession(apiClient, seedData, "Fresh Layout Task");
    await openTask(testPage, taskB.id);
    await expectNoTerminalDefault(testPage);
    await testPage.waitForTimeout(500);
    expect(await ordinaryShells(apiClient, taskB.id)).toHaveLength(0);

    await openTask(testPage, taskA.id);
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.getByTestId("layout-reset-item").click();
    await expectNoTerminalDefault(testPage);

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage
      .locator('[data-testid="layout-saved-delete"][data-layout-id="focused-default"]')
      .click();
    await expect(testPage.getByRole("alertdialog")).toContainText(
      "The built-in Default layout will become the default.",
    );
    await testPage.getByRole("button", { name: "Cancel" }).click();
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toHaveLength(1);
  });
});
