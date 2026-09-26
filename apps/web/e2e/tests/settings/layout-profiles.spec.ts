import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
import {
  expectApproxWidth,
  getDockviewContainerWidth,
  getDockviewGroupWidth,
} from "../../helpers/dockview-resize";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PR_NUMBER = 702;

type DockviewSnapshot = {
  panelIds: string[];
  filesGroupId: string | null;
  changesGroupId: string | null;
  prDetailsGroupId: string | null;
  rightGroupOrder: string[];
};

function noTerminalLayout() {
  return {
    columns: [
      {
        id: "center",
        width: 1050,
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

async function seedAndLinkMockPR(apiClient: ApiClient, taskId: string): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs([
    {
      number: PR_NUMBER,
      title: "Configured review placement",
      state: "open",
      head_branch: "feat/configured-review-placement",
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "testorg",
      repo_name: "testrepo",
      additions: 4,
      deletions: 1,
    },
  ]);
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: "testorg",
    repo: "testrepo",
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/testorg/testrepo/pull/${PR_NUMBER}`,
    pr_title: "Configured review placement",
    head_branch: "feat/configured-review-placement",
    base_branch: "main",
    author_login: "test-user",
    additions: 4,
    deletions: 1,
  });
}

async function dockviewSnapshot(page: Page): Promise<DockviewSnapshot> {
  return page.evaluate(() => {
    type Panel = { id: string; group?: { id: string; panels: Panel[] } };
    type Api = { panels: Panel[]; getPanel: (id: string) => Panel | undefined };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) throw new Error("dockview api not exposed");
    const files = api.getPanel("files");
    const changes = api.getPanel("changes");
    const prDetails = api.getPanel("pr-detail");
    return {
      panelIds: api.panels.map((panel) => panel.id),
      filesGroupId: files?.group?.id ?? null,
      changesGroupId: changes?.group?.id ?? null,
      prDetailsGroupId: prDetails?.group?.id ?? null,
      rightGroupOrder: files?.group?.panels.map((panel) => panel.id) ?? [],
    };
  });
}

/**
 * Wait until the dockview layout for this session has been persisted with the
 * given panel in it.
 *
 * The layout write is debounced (~350ms) and publishes nothing, so the only
 * honest anchor is the artifact it produces: `sessionStorage` under
 * `kandev.dockview.env-layout-v3.<envId>`. Polling the artifact rather than
 * sleeping past the debounce means a persist that never happens fails here
 * instead of silently changing what a later step is testing. It cannot be
 * satisfied by a stale write either: the panel is only in the layout after the
 * preset that adds it has been applied.
 */
async function waitForPersistedLayoutWithPanel(
  page: Page,
  sessionId: string,
  panelId: string,
): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(
          ({ sid, pid }) => {
            type StoreWindow = Window & {
              __KANDEV_E2E_STORE__?: {
                getState: () => { environmentIdBySessionId: Record<string, string> };
              };
            };
            const store = (window as StoreWindow).__KANDEV_E2E_STORE__;
            const envId = store?.getState().environmentIdBySessionId[sid];
            if (!envId) return false;
            const raw = window.sessionStorage.getItem(`kandev.dockview.env-layout-v3.${envId}`);
            return raw !== null && raw.includes(pid);
          },
          { sid: sessionId, pid: panelId },
        ),
      { timeout: 10_000, message: `layout for session ${sessionId} never persisted ${panelId}` },
    )
    .toBe(true);
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
  const containerWidth = await getDockviewContainerWidth(page);
  const rightWidth = await getDockviewGroupWidth(page, "files");
  expectApproxWidth(rightWidth, Math.round(containerWidth * 0.25), 12);
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
    seedData,
  }) => {
    test.setTimeout(120_000);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await testPage.getByRole("button", { name: "Add panel" }).hover();
    await expect(
      testPage.getByRole("tooltip", {
        name: "Add a missing tool tab beside the selected split.",
      }),
    ).toBeVisible();

    await layouts.selectPanel("Terminal");
    await layouts.actions.getByRole("button", { name: "Remove panel" }).hover();
    await expect(
      testPage.getByRole("tooltip", { name: "Remove this tab from the saved layout." }),
    ).toBeVisible();
    await layouts.removePanel("Terminal");
    await expect(testPage.getByRole("textbox", { name: "Layout profile name" })).toHaveCount(0);
    await expect(testPage.getByText("No custom profiles", { exact: true })).toBeVisible();
    await layouts.save();

    const response = await apiClient.getUserSettings();
    const saved = response.settings.saved_layouts;
    expect(saved).toHaveLength(1);
    expect(saved[0]).toMatchObject({
      id: "layout-override-default",
      name: "Default",
      is_default: true,
    });
    expect(JSON.stringify(saved[0].layout)).not.toContain("terminal-default");

    const task = await createTaskWithSession(apiClient, seedData, "Hidden Layout Override Task");
    await openTask(testPage, task.id);
    await testPage.getByTestId("layout-preset-trigger").click();
    await expect(
      testPage.locator(
        '[data-testid="layout-saved-delete"][data-layout-id="layout-override-default"]',
      ),
    ).toHaveCount(0);
    await testPage.locator('[data-testid="layout-preset-item"][data-preset-id="default"]').click();
    await expect(testPage.getByTestId("terminal-panel")).toHaveCount(0);
  });

  test("resets a customized built-in layout to its original definition", async ({
    testPage,
    apiClient,
  }) => {
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.removePanel("Terminal");
    await layouts.save();
    await testPage.getByRole("button", { name: "Reset built-in layout" }).click();
    await expect(testPage.getByText("Customized", { exact: true })).toHaveCount(0);
    await expect(layouts.editor.locator(".dv-tab", { hasText: "Terminal" })).toBeVisible();
    await layouts.save();

    expect((await apiClient.getUserSettings()).settings.saved_layouts).toEqual([]);
  });

  test("moves PR Details through the editor and uses the saved group for fresh tasks", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await expect(layouts.editor.locator(".dv-tab", { hasText: "PR Details" })).toHaveCount(0);
    await layouts.addPanel("PR Details");
    await expect(layouts.editor.locator(".dv-tab", { hasText: "PR Details" })).toBeVisible();
    await layouts.selectPanel("PR Details");
    await prCapture.screenshot("default-pr-details-agent-group", {
      caption: "The layout editor adds PR Details beside Agent in the center pane",
    });

    await layouts.actions.getByRole("button", { name: "Move tab to split" }).click();
    await testPage.getByRole("menuitem", { name: "Files", exact: true }).click();
    await prCapture.screenshot("pr-details-moved-to-files-group", {
      caption: "Layout editor moves PR Details into the Files/Changes pane before saving",
    });
    await layouts.save();

    const saved = (await apiClient.getUserSettings()).settings.saved_layouts[0];
    expect(JSON.stringify(saved.layout)).toContain("pr-detail");

    const task = await createTaskWithSession(apiClient, seedData, "Moved PR Details Layout Task");
    const session = await openTask(testPage, task.id);
    await expect
      .poll(() => dockviewSnapshot(testPage), { timeout: 15_000 })
      .toMatchObject({
        prDetailsGroupId: null,
        rightGroupOrder: ["files", "changes"],
      });

    await seedAndLinkMockPR(apiClient, task.id);
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => dockviewSnapshot(testPage), { timeout: 15_000 })
      .toMatchObject({
        filesGroupId: "group-right-top",
        changesGroupId: "group-right-top",
        prDetailsGroupId: "group-right-top",
        rightGroupOrder: ["files", "changes", "pr-detail"],
      });

    const agentGroup = testPage.locator('.dv-groupview:has([data-testid^="session-tab-"])');
    await agentGroup.getByTestId("dockview-add-panel-btn").click();
    await expect(
      testPage.getByTestId(`add-panel-pr-item-testorg-testrepo-${PR_NUMBER}`),
    ).toHaveCount(0);
    await expect(testPage.getByTestId("add-panel-pr-submenu")).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    await expect
      .poll(() => dockviewSnapshot(testPage))
      .toMatchObject({
        prDetailsGroupId: "group-right-top",
        rightGroupOrder: ["files", "changes", "pr-detail"],
      });
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
    // Task A's layout has to be on disk before the default changes underneath
    // it, or the later "existing tasks keep their terminal" assertion is
    // testing the new default rather than A's saved layout.
    await waitForPersistedLayoutWithPanel(testPage, taskA.session_id!, "terminal-default");

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
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "asserts that no ordinary shell is ever spawned for a task whose default layout has no terminal; a shell that must not be created publishes nothing, so the check needs real time to have any chance of catching one",
    );
    expect(await ordinaryShells(apiClient, taskB.id)).toHaveLength(0);

    await openTask(testPage, taskA.id);
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.getByTestId("layout-reset-item").click();
    await expectNoTerminalDefault(testPage);

    await openTask(testPage, taskA.id);
    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage
      .locator('[data-testid="layout-saved-delete"][data-layout-id="focused-default"]')
      .click({ force: true });
    const confirmation = testPage.getByTestId("layout-saved-delete-confirm-popover");
    await expect(confirmation).toContainText(
      "The built-in Default layout will become the default.",
    );
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toHaveLength(1);
  });

  test("confirms custom layout-profile deletion at the selected profile action", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.saveUserSettings({
      saved_layouts: [
        {
          id: "custom-layout-to-delete",
          name: "Custom layout to delete",
          is_default: true,
          layout: noTerminalLayout(),
          created_at: new Date().toISOString(),
        },
      ],
    });

    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    const deleteButton = testPage.getByTestId("layout-profile-delete");
    await expect(deleteButton).toBeVisible();
    await deleteButton.click();

    const confirmation = testPage.getByTestId("layout-profile-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toHaveLength(1);

    await deleteButton.click();
    await testPage
      .getByTestId("layout-profile-delete-confirm-popover")
      .getByTestId("layout-profile-delete-confirm")
      .click();
    await expect(deleteButton).toHaveCount(0);

    await layouts.save();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.saved_layouts)
      .toEqual([]);
  });

  test("adds Prompt History through the layout editor and restores it into a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();

    await expect(layouts.editor.locator(".dv-tab", { hasText: "Prompt History" })).toHaveCount(0);
    await layouts.addPanel("Prompt History");

    // The added panel survives save and is persisted in the default profile.
    await layouts.save();
    const saved = (await apiClient.getUserSettings()).settings.saved_layouts;
    expect(saved).toHaveLength(1);
    expect(JSON.stringify(saved[0].layout)).toContain("prompt-history");

    // A new task opens with the edited default layout; the panel restores and renders.
    const task = await createTaskWithSession(apiClient, seedData, "Prompt History Layout Task");
    await openTask(testPage, task.id);
    const tab = testPage.locator(".dv-tab", { hasText: "Prompt History" });
    await expect(tab).toBeVisible({ timeout: 15_000 });
    await tab.click();
    await expect(testPage.getByTestId("prompt-history-panel")).toBeVisible();
  });
});
