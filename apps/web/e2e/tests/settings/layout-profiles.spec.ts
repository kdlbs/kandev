import { expect, type Page } from "@playwright/test";
import type { DockviewApi } from "dockview-react";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";
import { SessionPage } from "../../pages/session-page";
import type { AppState } from "@/lib/state/store";
import { dwell } from "../../helpers/causal-waits";
import {
  installFixturePlugin,
  PLUGIN_ID,
  uninstallFixturePlugin,
} from "../../helpers/plugin-fixture";
import {
  NOTES_ID,
  NOTES_TITLE,
  PROMPT_ID,
  PROMPT_PSEUDO_TITLE,
  PROMPT_TITLE,
  dragRootSash,
  filesGroupPluginLayout,
  expectRatio,
  hasManualRightWidth,
  horizontalPluginLayout,
  panelPlacement,
  pluginPanel,
  rootSizes,
  savedProfile,
  setLocale,
  unloadPluginInPage,
  verticalPluginLayout,
} from "../../helpers/plugin-layout";
import {
  expectApproxWidth,
  getDockviewContainerWidth,
  getDockviewGroupWidth,
} from "../../helpers/dockview-resize";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PR_NUMBER = 702;
const RETIRED_DEFAULT_PROFILE_ID = "retired-panel-default";

/**
 * A default profile written before the Prompt history panel was removed: it
 * still references the retired panel as the only panel of its own group, next
 * to canonical panels that must keep restoring.
 */
function retiredPanelLayout() {
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
              { id: "todos", component: "todos", title: "Todos" },
            ],
            activePanel: "chat",
          },
          {
            id: "group-retired",
            panels: [
              { id: "prompt-history", component: "prompt-history", title: "Prompt History" },
            ],
            activePanel: "prompt-history",
          },
        ],
      },
      {
        id: "right",
        pinned: true,
        width: 350,
        groups: [
          {
            id: "group-right",
            panels: [{ id: "plan", component: "plan", title: "Plan" }],
            activePanel: "plan",
          },
        ],
      },
    ],
  };
}

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

async function serializedRootSizes(page: Page): Promise<number[]> {
  return page.evaluate(() => {
    const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
    const root = api?.toJSON().grid.root;
    if (root?.type !== "branch" || !Array.isArray(root.data))
      throw new Error("Dockview root branch not available");
    return root.data.map((child) => child.size);
  });
}

/**
 * The Todos panel is preference-gated at runtime (`show_todo_list_panel`
 * defaults to false), so the workbench removes a saved `todos` tab unless the
 * user has the panel enabled. A layout round trip that asserts the tab must
 * turn the preference on first.
 */
async function enableTodoListPanel(apiClient: ApiClient): Promise<void> {
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    show_todo_list_panel: true,
  });
  expect(response.ok).toBe(true);
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

  test("adds Todos through the layout editor and restores it into a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();

    await expect(layouts.editor.locator(".dv-tab", { hasText: "Todos" })).toHaveCount(0);
    await layouts.addPanel("Todos");

    // The added panel survives save and is persisted in the default profile.
    await layouts.save();
    const saved = (await apiClient.getUserSettings()).settings.saved_layouts;
    expect(saved).toHaveLength(1);
    expect(JSON.stringify(saved[0].layout)).toContain("todos");

    // A new task opens with the edited default layout; the panel restores and renders.
    await enableTodoListPanel(apiClient);
    const task = await createTaskWithSession(apiClient, seedData, "Todos Layout Task");
    const session = await openTask(testPage, task.id);
    const tab = testPage.locator(".dv-tab", { hasText: "Todos" });
    await expect(tab).toBeVisible({ timeout: 15_000 });
    await tab.click();
    // The panel root's test id is `todos-panel` when it has entries and
    // `todos-panel-empty-state` when it does not; this task's mock agent
    // produces none, so match the panel surface either way.
    await expect(testPage.getByTestId(/^todos-panel/)).toBeVisible();
    await session.expectLayoutHealthy();
  });

  test("opens a task whose saved default profile still references the retired panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({
      saved_layouts: [
        {
          id: RETIRED_DEFAULT_PROFILE_ID,
          name: "Retired panel default",
          is_default: true,
          layout: retiredPanelLayout(),
          created_at: new Date().toISOString(),
        },
      ],
    });

    // Settings first: the profile must keep applying, so it still carries the
    // default badge and is never demoted to an unavailable legacy record.
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    const profileRow = testPage.getByTestId(`layout-profile-custom-${RETIRED_DEFAULT_PROFILE_ID}`);
    await expect(profileRow).toBeVisible();
    await expect(profileRow.getByText("Default", { exact: true })).toBeVisible();
    await expect(profileRow.getByText("Unavailable", { exact: true })).toHaveCount(0);

    // Then the task: the retired entry is dropped and every seeded panel is
    // present, sized, and usable.
    await enableTodoListPanel(apiClient);
    const task = await createTaskWithSession(apiClient, seedData, "Retired Panel Layout Task");
    const session = await openTask(testPage, task.id);
    await expect(testPage.locator(".dv-tab", { hasText: "Prompt History" })).toHaveCount(0);
    await expect(testPage.locator(".dv-tab", { hasText: "Plan" })).toBeVisible({
      timeout: 15_000,
    });
    const todosTab = testPage.locator(".dv-tab", { hasText: "Todos" });
    await expect(todosTab).toBeVisible();
    await todosTab.click();
    await expect(testPage.getByTestId(/^todos-panel/)).toBeVisible();
    await session.expectLayoutHealthy();
  });
});

type SavedPanel = { id: string; title: string; params?: Record<string, unknown> };
type SavedLayoutPayload = {
  rootOrientation?: string;
  columns: Array<{
    id: string;
    pinned?: boolean;
    width?: number;
    groups: Array<{ id?: string; panels: SavedPanel[]; activePanel?: string }>;
    tree?: unknown;
  }>;
};

async function savedLayout(apiClient: ApiClient, id: string): Promise<SavedLayoutPayload> {
  const profile = (await apiClient.getUserSettings()).settings.saved_layouts.find(
    (candidate) => candidate.id === id,
  );
  if (!profile) throw new Error(`saved layout ${id} is missing`);
  return profile.layout as unknown as SavedLayoutPayload;
}

function savedPanel(layout: SavedLayoutPayload, panelId: string): SavedPanel | undefined {
  return layout.columns
    .flatMap((column) => column.groups)
    .flatMap((group) => group.panels)
    .find((panel) => panel.id === panelId);
}

async function setPluginEnabled(page: Page, enabled: boolean): Promise<void> {
  await page.goto("/settings/plugins");
  const row = page.getByTestId(`plugin-row-${PLUGIN_ID}`);
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: enabled ? "Enable" : "Disable" }).click();
  await expect(row.getByText(enabled ? "Active" : "Disabled", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
}

test.describe("Plugin task panels in reusable layout profiles", () => {
  test.afterEach(async ({ testPage, apiClient }) => {
    await setLocale(testPage, "en");
    await uninstallFixturePlugin(apiClient);
  });

  test("adds a plugin panel to the selected split, saves the built-in override, and applies it to a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();

    await layouts.selectPanel("Files");
    await layouts.addPanel(PROMPT_TITLE);
    const filesGroup = layouts.editor.locator(".dv-groupview", {
      has: testPage.locator(".dv-tab", { hasText: "Files" }),
    });
    await expect(filesGroup.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    await layouts.save();

    await testPage.reload();
    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    await layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
    await expect(testPage.getByTestId(`layout-placeholder-${PROMPT_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toHaveCount(0);

    const saved = await savedLayout(apiClient, "layout-override-default");
    expect(savedPanel(saved, PROMPT_ID)).toEqual(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    const savedGroup = saved.columns
      .flatMap((column) => column.groups)
      .find((group) => group.panels.some((panel) => panel.id === PROMPT_ID));
    expect(savedGroup?.panels.map((panel) => panel.id)).toEqual(["files", "changes", PROMPT_ID]);

    const task = await createTaskWithSession(apiClient, seedData, "Plugin Override Layout Task");
    await openTask(testPage, task.id);
    // The Todos tab is preference-gated runtime content that earlier specs in this worker can enable.
    await expect
      .poll(async () =>
        (await panelPlacement(testPage, PROMPT_ID))?.groupPanelIds.filter((id) => id !== "todos"),
      )
      .toEqual(["files", "changes", PROMPT_ID]);
    await testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible({
      timeout: 15_000,
    });
  });

  test("offers plugin-only choices and omits identities that are already placed", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    const pluginTitles = [NOTES_TITLE, PROMPT_TITLE];

    let choices = await layouts.addPanelChoices();
    expect(choices).toEqual(expect.arrayContaining(pluginTitles));
    for (let guard = 0; guard < 12; guard += 1) {
      const builtIn = choices.find((name) => !pluginTitles.includes(name));
      if (!builtIn) break;
      await layouts.addPanel(builtIn);
      choices = await layouts.addPanelChoices();
    }
    expect([...choices].sort()).toEqual([...pluginTitles].sort());

    await layouts.addPanel(PROMPT_TITLE);
    expect(await layouts.addPanelChoices()).toEqual([NOTES_TITLE]);

    await layouts.addPanel(NOTES_TITLE);
    await expect(
      testPage.getByTestId("layout-editor-add-panel").getByRole("button", { name: "Add panel" }),
    ).toBeDisabled();
  });

  test("keeps the saved placement editable while the plugin is unavailable and offers it again on re-enable", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(150_000);
    await installFixturePlugin(testPage);
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel("Files");
    await layouts.addPanel(PROMPT_TITLE);
    await layouts.save();
    const before = (await apiClient.getUserSettings()).settings.saved_layouts;

    await setPluginEnabled(testPage, false);
    await layouts.open();

    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    await expect(testPage.getByText("Unavailable", { exact: true })).toHaveCount(0);
    expect(await layouts.addPanelChoices()).not.toContain(PROMPT_TITLE);
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toEqual(before);
    await layouts.selectPanel("Files");
    await layouts.moveSelectedTabRight();
    await layouts.save();
    const edited = await savedLayout(apiClient, "layout-override-default");
    expect(savedPanel(edited, PROMPT_ID)).toEqual(pluginPanel(PROMPT_ID, PROMPT_TITLE));

    await setPluginEnabled(testPage, true);
    await layouts.open();
    await layouts.selectPanel("Files");
    expect(await layouts.addPanelChoices()).toContain(NOTES_TITLE);
    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
  });

  test("saves the canonical plugin title after the registration is revoked in the mounted editor", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    const layout = filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("layout-override-default", "Default", layout)],
    });
    await setLocale(testPage, "pseudo");
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    const pluginTab = layouts.editor.locator(".dv-tab", { hasText: PROMPT_PSEUDO_TITLE });
    await expect(pluginTab).toBeVisible();

    await unloadPluginInPage(testPage);

    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);

    const filesGroup = layouts.editor.locator(".dv-groupview", {
      has: testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }),
    });
    await filesGroup.locator(".dv-tab").first().click();
    await expect(layouts.actions).toBeVisible();
    await layouts.actions.getByRole("button").nth(1).click();
    await layouts.saveLocalized();

    const saved = await savedLayout(apiClient, "layout-override-default");
    expect(savedPanel(saved, PROMPT_ID)).toEqual(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    const group = saved.columns
      .flatMap((column) => column.groups)
      .find((candidate) => candidate.panels.some((panel) => panel.id === PROMPT_ID));
    expect(group?.panels.map((panel) => panel.id)).toEqual(["changes", "files", PROMPT_ID]);

    await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
    await testPage.reload();
    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    const unavailableGroup = layouts.editor.locator(".dv-groupview", {
      has: testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }),
    });
    await unavailableGroup.locator(".dv-tab").first().click();
    await layouts.actions.getByRole("button").nth(1).click();
    await layouts.saveLocalized();
    expect(savedPanel(await savedLayout(apiClient, "layout-override-default"), PROMPT_ID)).toEqual(
      pluginPanel(PROMPT_ID, PROMPT_TITLE),
    );
  });
});

test.describe("Plugin panel geometry in applied profiles", () => {
  test.afterEach(async ({ apiClient }) => uninstallFixturePlugin(apiClient));

  for (const profileId of ["layout-override-default", "horizontal-plugin"]) {
    test(`preserves ${profileId} geometry when reopened, edited, and saved again`, async ({
      testPage,
      apiClient,
      seedData,
      prCapture,
    }) => {
      await installFixturePlugin(testPage);
      test.setTimeout(180_000);
      await testPage.setViewportSize({ width: 900, height: 900 });
      await apiClient.saveUserSettings({
        saved_layouts: [
          savedProfile(
            profileId,
            "Horizontal plugin",
            horizontalPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE), {
              center: 960,
              right: 240,
            }),
          ),
        ],
      });
      const layouts = new LayoutSettingsPage(testPage);
      await layouts.open();
      expect((await layouts.editor.boundingBox())?.width).toBeLessThan(900);
      await layouts.selectPanel("Files");
      await layouts.moveSelectedTabRight();
      await layouts.save();

      await testPage.reload();
      await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
      const saved = await savedLayout(apiClient, profileId);
      const [center, right] = saved.columns.map((column) => column.width ?? 0);
      expectRatio(right, center + right, 0.2, 0.01);
      expect(saved.columns[1].groups[0].panels.map((panel) => panel.id)).toEqual([
        "changes",
        "files",
      ]);
      await testPage.setViewportSize({ width: 1600, height: 900 });
      const task = await createTaskWithSession(apiClient, seedData, `Resaved ${profileId}`);
      await openTask(testPage, task.id);
      await expect
        .poll(async () => {
          const sizes = await rootSizes(testPage);
          return Math.abs(sizes[1] / (sizes[0] + sizes[1]) - 0.2);
        })
        .toBeLessThan(0.01);
      await testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
      await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
      await prCapture.screenshot(`resaved-source-geometry-${profileId}`, {
        caption: "Wide task retains source proportions after a constrained settings edit",
      });
    });
  }

  test("resizes every right group created by the settings split action", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    const source = filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    source.columns.forEach((column) => {
      column.width = 800;
    });
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("generated-plugin", "Generated group geometry", source)],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel(PROMPT_TITLE);
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Below", exact: true }).click();
    await layouts.save();
    await testPage.reload();
    const saved = await savedLayout(apiClient, "generated-plugin");
    const right = saved.columns[1];
    const pluginGroup = right.groups.find((group) =>
      group.panels.some((panel) => panel.id === PROMPT_ID),
    );
    expect(right.groups).toHaveLength(2);
    expect(pluginGroup?.id).toBeDefined();
    expect(pluginGroup?.id).not.toBe("group-right-bottom");
    const share = right.width! / saved.columns.reduce((sum, column) => sum + column.width!, 0);
    expect(share).toBeGreaterThan(0.25);
    await testPage.setViewportSize({ width: 1120, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Generated right groups");
    await openTask(testPage, task.id);
    const narrowWidth = await getDockviewContainerWidth(testPage);
    expectApproxWidth(narrowWidth, 800, 3);
    await expect
      .poll(async () => (await rootSizes(testPage))[1])
      .toBeCloseTo(Math.min(Math.round(share * narrowWidth), narrowWidth - 480), 0);
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    for (const viewport of [1920, 1120, 1920]) {
      await testPage.setViewportSize({ width: viewport, height: 900 });
      const expectedContainer = viewport - 320;
      await expect
        .poll(() => getDockviewContainerWidth(testPage))
        .toBeCloseTo(expectedContainer, 0);
      const expected = Math.min(Math.round(share * expectedContainer), expectedContainer - 480);
      await expect.poll(async () => (await rootSizes(testPage))[1]).toBeCloseTo(expected, 0);
      const files = await panelPlacement(testPage, "files");
      const prompt = await panelPlacement(testPage, PROMPT_ID);
      expect(files?.groupId).not.toBe(prompt?.groupId);
      expectApproxWidth(files?.width ?? 0, expected, 2);
      expectApproxWidth(prompt?.width ?? 0, expected, 2);
    }
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    expect(await hasManualRightWidth(testPage)).toBe(false);
    await prCapture.screenshot("settings-generated-right-groups-expanded", {
      caption: "Settings-created plugin split expands with the owned reusable right column",
    });
  });

  test("preserves three-root geometry and the middle Files column's trailing sash width", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    const source = filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    source.columns.forEach((column) => {
      column.width = 800;
    });
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("three-root-plugin", "Three root geometry", source)],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel(PROMPT_TITLE);
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
    await layouts.save();
    await testPage.reload();
    const saved = await savedLayout(apiClient, "three-root-plugin");
    expect(saved.columns).toHaveLength(3);
    expect(saved.columns.map((column) => column.id)).toEqual(["center", "right", PROMPT_ID]);
    const share =
      saved.columns[1].width! / saved.columns.reduce((sum, column) => sum + column.width!, 0);
    await testPage.setViewportSize({ width: 1520, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Three root geometry and sash");
    const session = await openTask(testPage, task.id);
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    for (const viewport of [1520, 1920, 1520, 1920]) {
      await testPage.setViewportSize({ width: viewport, height: 900 });
      const width = viewport - 320;
      await expect.poll(() => getDockviewContainerWidth(testPage)).toBeCloseTo(width, 0);
      await expect
        .poll(async () => (await rootSizes(testPage))[1])
        .toBeCloseTo(Math.round(share * width), 0);
      const sizes = await rootSizes(testPage);
      expect(sizes).toHaveLength(3);
      const files = await panelPlacement(testPage, "files");
      const prompt = await panelPlacement(testPage, PROMPT_ID);
      expect(files?.groupId).not.toBe(prompt?.groupId);
      expectApproxWidth(files?.width ?? 0, sizes[1], 2);
      expectApproxWidth(prompt?.width ?? 0, sizes[2], 2);
    }
    expect(await hasManualRightWidth(testPage)).toBe(false);
    await dragRootSash(testPage, "last", 1, 500);
    const manualWidths = () =>
      testPage.evaluate(() =>
        Object.entries(window.sessionStorage)
          .filter(([key]) => key.startsWith("kandev.dockview.manual-right-width-v1."))
          .map(([, value]) => JSON.parse(value) as number),
      );
    await expect.poll(manualWidths).toEqual([500]);
    await testPage.locator(".dv-tab", { hasText: "Changes" }).click();
    await expect.poll(async () => (await rootSizes(testPage))[1]).toBe(500);
    for (const viewport of [1520, 1920]) {
      await testPage.setViewportSize({ width: viewport, height: 900 });
      await expect.poll(() => getDockviewContainerWidth(testPage)).toBeCloseTo(viewport - 320, 0);
      await expect.poll(async () => (await rootSizes(testPage))[1]).toBe(500);
      expectApproxWidth(await getDockviewGroupWidth(testPage, "files"), 500, 2);
    }
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    await prCapture.screenshot("settings-three-root-trailing-sash", {
      caption: "Settings Right split keeps three root groups and a trailing-sash manual width",
    });
    const awayTitle = "Three root saved restore neighbor";
    const away = await createTaskWithSession(apiClient, seedData, awayTitle);
    await session.clickTaskInSidebar(awayTitle);
    await expect(testPage).toHaveURL(new RegExp(`/t/${away.id}`));
    await session.waitForDockviewReady();
    await expect
      .poll(() => getDockviewGroupWidth(testPage, "files"))
      .toBeCloseTo(Math.round(share * 1600), 0);
    await session.clickTaskInSidebar("Three root geometry and sash");
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    await session.waitForDockviewReady();
    for (const restoration of ["task-return", "reload"]) {
      if (restoration === "reload") {
        await testPage.reload();
        await session.waitForLoad();
        await session.waitForDockviewReady();
      }
      await expect.poll(manualWidths).toEqual([500]);
      await expect.poll(() => getDockviewGroupWidth(testPage, "files")).toBe(500);
      expect(await rootSizes(testPage)).toHaveLength(3);
      for (const viewport of [1520, 1920]) {
        await testPage.setViewportSize({ width: viewport, height: 900 });
        await expect.poll(() => getDockviewContainerWidth(testPage)).toBeCloseTo(viewport - 320, 0);
        await expect.poll(() => getDockviewGroupWidth(testPage, "files")).toBe(500);
        expect(await rootSizes(testPage)).toHaveLength(3);
      }
      await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
      await prCapture.screenshot(`settings-three-root-${restoration}`, {
        caption:
          "Saved task restores the middle Files manual width independently of reusable ratios",
      });
    }
  });

  test("restores all root weights after cap release for a settings-created Right split", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    await apiClient.saveUserSettings({
      saved_layouts: [
        savedProfile(
          "root-cap-release",
          "Root cap release",
          filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE)),
        ),
      ],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel(PROMPT_TITLE);
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
    await layouts.save();
    const captured = await savedLayout(apiClient, "root-cap-release");
    expect(captured.columns.map((column) => column.id)).toEqual(["center", "right", PROMPT_ID]);
    const serializedSizes = () => serializedRootSizes(testPage);
    for (const geometry of [
      { widths: [400, 1000, 200], narrow: [320, 320, 160], wide: 1600 },
      { widths: [800, 1200, 800], narrow: [240, 320, 240], wide: 2800 },
    ]) {
      const source = structuredClone(captured);
      source.columns.forEach((column, index) => {
        column.width = geometry.widths[index];
      });
      await apiClient.saveUserSettings({
        saved_layouts: [savedProfile("root-cap-release", "Root cap release", source)],
      });
      await testPage.setViewportSize({ width: 1120, height: 900 });
      const task = await createTaskWithSession(
        apiClient,
        seedData,
        `Root cap release ${geometry.wide}`,
      );
      await openTask(testPage, task.id);
      for (const width of [800, geometry.wide, 800, geometry.wide]) {
        await testPage.setViewportSize({ width: width + 320, height: 900 });
        await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(width);
        const expected = width === 800 ? geometry.narrow : geometry.widths;
        await expect.poll(() => rootSizes(testPage)).toEqual(expected);
        await expect.poll(serializedSizes).toEqual(expected);
      }
      await testPage.locator(".dv-tab", { hasText: "Files" }).click();
      await expect(testPage.getByTestId("files-panel")).toBeVisible();
      await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
      expect(await hasManualRightWidth(testPage)).toBe(false);
      expect(
        (await savedLayout(apiClient, "root-cap-release")).columns.map((column) => column.width),
      ).toEqual(geometry.widths);
      await prCapture.screenshot(`settings-root-cap-release-${geometry.wide}`, {
        caption:
          "Cap release restores every saved root proportion without changing the portable source",
      });
    }
  });

  test("round-trips distinct Files and Changes roots created by settings Split Right", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    await apiClient.saveUserSettings({
      saved_layouts: [
        savedProfile(
          "tool-roots",
          "Tool roots",
          filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE)),
        ),
      ],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel("Changes");
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
    await layouts.save();
    const source = await savedLayout(apiClient, "tool-roots");
    source.columns.forEach((column, index) => {
      column.width = [400, 800, 400][index];
    });
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("tool-roots", "Tool roots", source)],
    });
    await testPage.reload();
    await expect(layouts.editor.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    expect(await savedLayout(apiClient, "tool-roots")).toEqual(source);
    await testPage.setViewportSize({ width: 1120, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Distinct tool roots");
    await openTask(testPage, task.id);
    for (const width of [1600, 800, 1600, 800, 1600]) {
      await testPage.setViewportSize({ width: width + 320, height: 900 });
      await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(width);
      const expected = width === 800 ? [240, 320, 240] : [400, 800, 400];
      await expect.poll(() => rootSizes(testPage)).toEqual(expected);
      await expect.poll(() => serializedRootSizes(testPage)).toEqual(expected);
      const caps = await testPage.evaluate(() => {
        const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
        return {
          files: api?.getPanel("files")?.group.maximumWidth,
          changes: api?.getPanel("changes")?.group.maximumWidth,
        };
      });
      expect(caps.files).toBe(width === 800 ? 320 : 1120);
      expect(caps.changes).toBeGreaterThanOrEqual(400);
    }
    expect(new Set(source.columns.map((column) => column.id)).size).toBe(3);
    expect(source.columns.filter((column) => column.pinned)).toHaveLength(1);
    await testPage.locator(".dv-tab", { hasText: "Files" }).click();
    await expect(testPage.getByTestId("files-panel")).toBeVisible();
    await testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    expect(await hasManualRightWidth(testPage)).toBe(false);
    expect(await savedLayout(apiClient, "tool-roots")).toEqual(source);
    await prCapture.screenshot("settings-distinct-tool-roots", {
      caption:
        "JSON-reloaded Files and Changes roots retain independent proportions after cap release",
    });
  });

  for (const profileId of ["raw-custom", "layout-override-default"]) {
    test(`applies raw saved ${profileId} through the workbench selector without aliasing roots`, async ({
      testPage,
      apiClient,
      seedData,
      prCapture,
    }) => {
      test.setTimeout(180_000);
      await installFixturePlugin(testPage);
      await testPage.setViewportSize({ width: 1920, height: 900 });
      await apiClient.saveUserSettings({
        saved_layouts: [
          savedProfile(
            "raw-builder",
            "Raw builder",
            filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE)),
          ),
        ],
      });
      const layouts = new LayoutSettingsPage(testPage);
      await layouts.open();
      await layouts.selectPanel("Changes");
      await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
      await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
      await layouts.save();
      const source = await savedLayout(apiClient, "raw-builder");
      source.columns.forEach((column, index) => {
        column.width = [400, 800, 400][index];
      });
      source.columns[2].id = "right";
      source.columns[2].pinned = true;
      const name = "Raw saved tool roots";
      await apiClient.saveUserSettings({
        saved_layouts: [savedProfile(profileId, name, source, false)],
      });
      await testPage.reload();
      expect(await savedLayout(apiClient, profileId)).toEqual(source);
      await testPage.setViewportSize({ width: 1120, height: 900 });
      const task = await createTaskWithSession(apiClient, seedData, `Raw selector ${profileId}`);
      await openTask(testPage, task.id);
      await testPage.getByTestId("layout-preset-trigger").click();
      if (profileId === "layout-override-default") {
        await testPage
          .locator('[data-testid="layout-preset-item"][data-preset-id="default"]')
          .click();
      } else {
        await testPage.getByRole("menuitem", { name, exact: true }).click();
      }
      for (const width of [800, 1600, 800, 1600]) {
        await testPage.setViewportSize({ width: width + 320, height: 900 });
        await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(width);
        const expected = width === 800 ? [240, 320, 240] : [400, 800, 400];
        await expect.poll(() => rootSizes(testPage)).toEqual(expected);
        await expect.poll(() => serializedRootSizes(testPage)).toEqual(expected);
      }
      await testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
      await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
      await testPage.locator(".dv-tab", { hasText: "Changes" }).click();
      await expect(testPage.getByTestId("changes-panel")).toBeVisible();
      expect(await savedLayout(apiClient, profileId)).toEqual(source);
      expect(await hasManualRightWidth(testPage)).toBe(false);
      await prCapture.screenshot(`settings-raw-selector-${profileId}`, {
        caption:
          "Explicit selection normalizes legacy duplicate roots without rewriting saved JSON",
      });
    });
  }

  test("releases ordinary source weights when the sole tool root is moved away and undone", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    const { settings } = await apiClient.getUserSettings();
    await apiClient.saveUserSettings({ show_todo_list_panel: false });
    try {
      await testPage.setViewportSize({ width: 1920, height: 900 });
      await apiClient.saveUserSettings({
        saved_layouts: [
          savedProfile(
            "ordinary-no-right",
            "Ordinary no right",
            filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE)),
          ),
        ],
      });
      const layouts = new LayoutSettingsPage(testPage);
      await layouts.open();
      await layouts.selectPanel(PROMPT_TITLE);
      await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
      await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
      await layouts.save();
      const source = await savedLayout(apiClient, "ordinary-no-right");
      source.columns.forEach((column, index) => {
        column.width = [400, 1000, 200][index];
        for (const group of column.groups) {
          group.panels = group.panels.filter((panel) => panel.id !== "changes");
          group.activePanel = group.panels[0].id;
        }
      });
      await apiClient.saveUserSettings({
        saved_layouts: [savedProfile("ordinary-no-right", "Ordinary no right", source)],
      });
      await testPage.setViewportSize({ width: 1120, height: 900 });
      const task = await createTaskWithSession(apiClient, seedData, "Ordinary move and undo");
      await openTask(testPage, task.id);
      await expect.poll(() => rootSizes(testPage)).toEqual([320, 320, 160]);
      await expect(testPage.locator(".dv-tab", { hasText: "Todos" })).toHaveCount(0);
      // The unrelated undefined-review serialization prerequisite remains normal UI setup.
      const fixtureReview = testPage.locator(".dv-tab", { hasText: "Bitbucket Pull Request #42" });
      await expect(fixtureReview).toHaveCount(1);
      await fixtureReview.hover();
      await fixtureReview.locator(".dv-default-tab-action").click();
      await expect(fixtureReview).toHaveCount(0);
      await testPage.evaluate(() => {
        const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
        const files = api?.getPanel("files");
        const agent = api?.panels.find((panel) => panel.api.component === "chat");
        if (!files || !agent) throw new Error("Move panels not available");
        files.api.moveTo({ group: agent.group, position: "center" });
      });
      await expect.poll(() => rootSizes(testPage)).toEqual([400, 400]);
      await testPage.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
      await testPage.evaluate(() => {
        const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
        const files = api?.getPanel("files");
        if (!files) throw new Error("Undo panel not available");
        files.api.moveTo({ group: files.group, position: "right" });
      });
      await expect.poll(async () => (await rootSizes(testPage)).length).toBe(3);
      await testPage.setViewportSize({ width: 1920, height: 900 });
      await expect.poll(() => rootSizes(testPage)).toEqual([500, 1000, 100]);
      await expect.poll(() => serializedRootSizes(testPage)).toEqual([500, 1000, 100]);
      await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
      expect(await hasManualRightWidth(testPage)).toBe(false);
      expect(await savedLayout(apiClient, "ordinary-no-right")).toEqual(source);
      await prCapture.screenshot("settings-ordinary-no-right-undo", {
        caption: "A genuine move releases all-root source weights even while no right root is live",
      });
    } finally {
      await apiClient.saveUserSettings({
        show_todo_list_panel: settings.show_todo_list_panel ?? false,
      });
    }
  });

  test("releases source weights after a real structural edit while pinned-last is hidden", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    await apiClient.saveUserSettings({
      saved_layouts: [
        savedProfile(
          "hidden-pinned-last",
          "Hidden pinned last",
          filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE)),
        ),
      ],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel(PROMPT_TITLE);
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Left", exact: true }).click();
    await layouts.save();
    const source = await savedLayout(apiClient, "hidden-pinned-last");
    source.columns.forEach((column, index) => {
      column.width = [400, 200, 1000][index];
    });
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("hidden-pinned-last", "Hidden pinned last", source)],
    });
    await testPage.setViewportSize({ width: 1120, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Hidden pinned root edit");
    await openTask(testPage, task.id);
    await expect.poll(() => rootSizes(testPage)).toEqual([320, 160, 320]);
    // Preserve the existing normal-UI prerequisite for the unrelated undefined-review serialization defect.
    const fixtureReview = testPage.locator(".dv-tab", { hasText: "Bitbucket Pull Request #42" });
    await expect(fixtureReview).toHaveCount(1);
    await fixtureReview.hover();
    await fixtureReview.locator(".dv-default-tab-action").click();
    await expect(fixtureReview).toHaveCount(0);
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await toggle.click();
    await expect.poll(() => panelPlacement(testPage, "files")).toBeNull();
    const agent = testPage.locator(".dv-tabs-and-actions-container").filter({
      has: testPage.getByTestId(`session-tab-${task.session_id}`),
    });
    await agent.getByTestId("dockview-add-panel-btn").click();
    await testPage.getByRole("menuitem", { name: "Plan", exact: true }).click();
    await expect(testPage.locator(".dv-tab", { hasText: "Plan" })).toBeVisible();
    // The exposed real Dockview API performs the same split/merge primitives as settings actions.
    await testPage.evaluate(() => {
      const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
      const plan = api?.getPanel("plan");
      if (!plan) throw new Error("Plan panel not available");
      plan.api.moveTo({ group: plan.group, position: "right" });
    });
    await expect.poll(async () => (await rootSizes(testPage)).length).toBe(3);
    await testPage.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await testPage.evaluate((pluginId) => {
      const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
      const plan = api?.getPanel("plan");
      const plugin = api?.getPanel(pluginId);
      if (!plan || !plugin) throw new Error("Split/merge panels not available");
      plan.group.api.moveTo({ group: plugin.group, position: "center" });
      plan.api.close();
    }, PROMPT_ID);
    await expect.poll(() => rootSizes(testPage)).toEqual([400, 400]);
    await toggle.click();
    await expect.poll(() => panelPlacement(testPage, "files")).not.toBeNull();
    await testPage.setViewportSize({ width: 1920, height: 900 });
    await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(1600);
    await expect.poll(() => rootSizes(testPage)).toEqual([480, 120, 1000]);
    await expect.poll(() => serializedRootSizes(testPage)).toEqual([480, 120, 1000]);
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    expect(await hasManualRightWidth(testPage)).toBe(false);
    expect(await savedLayout(apiClient, "hidden-pinned-last")).toEqual(source);
    await prCapture.screenshot("settings-hidden-pinned-last-edited", {
      caption:
        "Revealing Files honors the real hidden split/merge edit instead of reviving source weights",
    });
  });

  test("preserves three-root geometry through conditional tabs and retained pane lifecycle", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1920, height: 900 });
    const base = filesGroupPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    base.columns[0].groups[0].panels.push({ id: "todos", component: "todos", title: "Todos" });
    await apiClient.saveUserSettings({
      show_todo_list_panel: true,
      show_todo_list_panel_only_when_not_empty: false,
      saved_layouts: [savedProfile("root-lifecycle", "Root lifecycle", base)],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await layouts.selectPanel(PROMPT_TITLE);
    await layouts.actions.getByRole("button", { name: "Split tab", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Right", exact: true }).click();
    await layouts.save();
    const source = await savedLayout(apiClient, "root-lifecycle");
    source.columns.forEach((column, index) => {
      column.width = [400, 1000, 200][index];
    });
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("root-lifecycle", "Root lifecycle", source)],
    });
    await testPage.setViewportSize({ width: 1120, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Root lifecycle geometry");
    await openTask(testPage, task.id);
    await expect.poll(() => rootSizes(testPage)).toEqual([320, 320, 160]);
    await expect.poll(() => panelPlacement(testPage, "todos")).not.toBeNull();
    const serializedSizes = () => serializedRootSizes(testPage);
    for (const enabled of [false, true, false]) {
      await apiClient.saveUserSettings({ show_todo_list_panel: enabled });
      // Drive the shipped preference action, leaving the same workbench and geometry owner mounted.
      await testPage.evaluate((showTodoListPanel) => {
        const state = (
          window as Window & { __KANDEV_E2E_STORE__?: { getState: () => AppState } }
        ).__KANDEV_E2E_STORE__?.getState();
        if (!state) throw new Error("App store not exposed");
        state.setUserSettings({ ...state.userSettings, showTodoListPanel });
      }, enabled);
      await expect.poll(async () => !!(await panelPlacement(testPage, "todos"))).toBe(enabled);
      for (const width of [1600, 800]) {
        await testPage.setViewportSize({ width: width + 320, height: 900 });
        await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(width);
        const expected = width === 800 ? [320, 320, 160] : [400, 1000, 200];
        await expect.poll(() => rootSizes(testPage)).toEqual(expected);
        await expect.poll(serializedSizes).toEqual(expected);
      }
    }
    // Normal UI close isolates the pre-existing undefined-provider serialization boundary.
    const fixtureReview = testPage.locator(".dv-tab", { hasText: "Bitbucket Pull Request #42" });
    await expect(fixtureReview).toHaveCount(1);
    await fixtureReview.hover();
    await fixtureReview.locator(".dv-default-tab-action").click();
    await expect(fixtureReview).toHaveCount(0);
    const toggle = testPage.getByTestId("task-right-panels-toggle");
    await toggle.click();
    await expect.poll(() => panelPlacement(testPage, PROMPT_ID)).toBeNull();
    await testPage.setViewportSize({ width: 1920, height: 900 });
    await expect.poll(() => getDockviewContainerWidth(testPage)).toBe(1600);
    await expect.poll(() => rootSizes(testPage)).toEqual([600, 1000]);
    await expect.poll(serializedSizes).toEqual([600, 1000]);
    await toggle.click();
    await expect.poll(() => rootSizes(testPage)).toEqual([400, 1000, 200]);
    await expect.poll(serializedSizes).toEqual([400, 1000, 200]);
    await testPage.locator(".dv-tab", { hasText: "Files" }).click();
    await expect(testPage.getByTestId("files-panel")).toBeVisible();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible();
    expect(await hasManualRightWidth(testPage)).toBe(false);
    expect(await savedLayout(apiClient, "root-lifecycle")).toEqual(source);
    await prCapture.screenshot("settings-root-lifecycle-restored", {
      caption:
        "Conditional tab removal and hidden-root projection preserve every source proportion",
    });
  });

  test("applies a VERTICAL root profile with root-axis proportions, nested widths, and stable sash state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    await installFixturePlugin(testPage);
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("vertical-plugin", "Vertical plugin", verticalPluginLayout())],
    });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();
    await expect(
      testPage.getByTestId("layout-profile-custom-vertical-plugin").getByText("Unavailable", {
        exact: true,
      }),
    ).toHaveCount(0);
    await layouts.selectPanel("Files");
    await layouts.moveSelectedTabRight();
    await layouts.save();

    const saved = await savedLayout(apiClient, "vertical-plugin");
    expect(saved.rootOrientation).toBe("VERTICAL");
    expect(saved.columns.map((column) => column.pinned)).toEqual([undefined, undefined]);
    expect(saved.columns[1].groups[0].panels.map((panel) => panel.id)).toEqual([
      "changes",
      "files",
    ]);
    expect(savedPanel(saved, PROMPT_ID)).toEqual(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    const [savedRow0, savedRow1] = saved.columns.map((column) => column.width ?? 0);
    expectRatio(savedRow0, savedRow0 + savedRow1, 3 / 7, 0.05);

    await testPage.setViewportSize({ width: 1400, height: 900 });
    const task = await createTaskWithSession(apiClient, seedData, "Vertical Plugin Layout Task");
    await openTask(testPage, task.id);
    const expectVerticalGeometry = async (label: string) => {
      await expect
        .poll(
          async () => {
            const [top, bottom] = await rootSizes(testPage);
            return top / (top + bottom);
          },
          { message: `${label}: root rows keep their 3:4 proportion` },
        )
        .toBeGreaterThan(0.38);
      const [top, bottom] = await rootSizes(testPage);
      expectRatio(top, top + bottom, 3 / 7, 0.05);
      const files = await panelPlacement(testPage, "files");
      const prompt = await panelPlacement(testPage, PROMPT_ID);
      expect(files?.groupPanelIds, label).toEqual(["changes", "files"]);
      expect(prompt?.groupPanelIds, label).toEqual([PROMPT_ID]);
      expectRatio(files?.width ?? 0, (files?.width ?? 0) + (prompt?.width ?? 0), 0.4, 0.05);
      expect(await hasManualRightWidth(testPage), label).toBe(false);
    };
    await expectVerticalGeometry("fresh default");
    await testPage.locator(".dv-tab", { hasText: PROMPT_TITLE }).click();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.getByTestId("layout-reset-item").click();
    await expectVerticalGeometry("Reset Layout");

    await testPage.reload();
    await new SessionPage(testPage).waitForDockviewReady();
    await expectVerticalGeometry("task reload");

    await testPage.setViewportSize({ width: 1250, height: 760 });
    await expectVerticalGeometry("container resize");

    const [topRow] = await rootSizes(testPage);
    await dragRootSash(testPage, "first", 0, topRow - 60);
    expect((await rootSizes(testPage))[0]).toBeLessThan(topRow);
    expect(await hasManualRightWidth(testPage)).toBe(false);
  });

  test("keeps a horizontal profile's saved in-cap ratio through task navigation and respects a manual width", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(240_000);
    await installFixturePlugin(testPage);
    await testPage.setViewportSize({ width: 1600, height: 900 });
    const notesProfile = horizontalPluginLayout(pluginPanel(NOTES_ID, NOTES_TITLE));
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("horizontal-plugin", "Horizontal plugin", notesProfile)],
    });
    const savedEnvironment = await createTaskWithSession(apiClient, seedData, "Notes Environment");
    await openTask(testPage, savedEnvironment.id);
    await expect(testPage.getByTestId("e2e-notes-panel")).toBeVisible({ timeout: 15_000 });
    await testPage.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await expect
      .poll(
        async () => {
          const width = await getDockviewContainerWidth(testPage);
          return Math.abs(((await rootSizes(testPage)).at(-1) ?? 0) / width - 0.2);
        },
        { message: "cold-ready task keeps its reusable ratio on sidebar collapse" },
      )
      .toBeLessThan(0.01);
    await testPage.getByRole("button", { name: "Expand sidebar", exact: true }).click();
    await waitForPersistedLayoutWithPanel(testPage, savedEnvironment.session_id!, NOTES_ID);

    const promptProfile = horizontalPluginLayout(pluginPanel(PROMPT_ID, PROMPT_TITLE));
    await apiClient.saveUserSettings({
      saved_layouts: [savedProfile("horizontal-plugin", "Horizontal plugin", promptProfile)],
    });
    const freshEnvironment = await createTaskWithSession(apiClient, seedData, "Prompt Default");
    const session = new SessionPage(testPage);

    await session.sidebarTaskItem("Prompt Default").click();
    await expect.poll(() => panelPlacement(testPage, PROMPT_ID)).not.toBeNull();
    await expect(testPage.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
    expect(await panelPlacement(testPage, NOTES_ID)).toBeNull();
    const container = await getDockviewContainerWidth(testPage);
    await expect
      .poll(async () => (await rootSizes(testPage)).at(-1) ?? 0, {
        message: "right column settles at the saved 20% share, not the stock 30%",
      })
      .toBeLessThan(container * 0.25);
    const rightWidth = (await rootSizes(testPage)).at(-1) ?? 0;
    expectRatio(rightWidth, container, 0.2, 0.03);
    expect(await hasManualRightWidth(testPage)).toBe(false);

    const viewport = testPage.viewportSize();
    await testPage.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await expect.poll(() => getDockviewContainerWidth(testPage)).toBeGreaterThan(container);
    await expect
      .poll(async () => {
        const width = await getDockviewContainerWidth(testPage);
        const right = (await rootSizes(testPage)).at(-1) ?? 0;
        return Math.abs(right / width - 0.2);
      })
      .toBeLessThan(0.03);
    expect(testPage.viewportSize()).toEqual(viewport);
    await testPage.getByRole("button", { name: "Expand sidebar", exact: true }).click();
    await expect
      .poll(async () => Math.abs((await getDockviewContainerWidth(testPage)) - container))
      .toBeLessThan(1);
    await expect
      .poll(async () => {
        const right = (await rootSizes(testPage)).at(-1) ?? 0;
        return Math.abs(right / container - 0.2);
      })
      .toBeLessThan(0.03);
    // Exercise the configured Agent/Files/plugin layout without the fixture's incidental review tab.
    const fixtureReview = testPage.locator(".dv-tab", { hasText: "Bitbucket Pull Request #42" });
    await expect(fixtureReview).toHaveCount(1);
    await fixtureReview.hover();
    await fixtureReview.locator(".dv-default-tab-action").click();
    await expect(fixtureReview).toHaveCount(0);
    const maximizeAgent = testPage
      .locator(".dv-tabs-and-actions-container")
      .filter({
        has: testPage.getByTestId(`session-tab-${freshEnvironment.session_id}`),
      })
      .getByTestId("dockview-maximize-btn");
    for (const action of ["hide", "maximize"]) {
      const toggle = testPage.getByTestId("task-right-panels-toggle");
      if (action === "hide") await toggle.click();
      else await maximizeAgent.click();
      await expect(testPage.locator(".dv-tab", { hasText: PROMPT_TITLE })).toHaveCount(0);
      await testPage.setViewportSize({ width: 1900, height: 900 });
      await expect.poll(() => getDockviewContainerWidth(testPage)).toBeGreaterThan(container + 250);
      if (action === "hide") await expect(toggle).toBeEnabled();
      if (action === "hide") await toggle.click();
      else await maximizeAgent.click();
      await expect
        .poll(async () => {
          const width = await getDockviewContainerWidth(testPage);
          return Math.abs(((await rootSizes(testPage)).at(-1) ?? 0) / width - 0.2);
        })
        .toBeLessThan(0.01);
      await expect(testPage.locator(".dv-tab", { hasText: PROMPT_TITLE })).toBeVisible();
      expect(await hasManualRightWidth(testPage)).toBe(false);
      await prCapture.screenshot(`restored-source-geometry-${action}`, {
        caption: "Restored right pane resolves the source share at the resized container",
      });
      await testPage.setViewportSize({ width: 1600, height: 900 });
      await expect
        .poll(async () => Math.abs(((await rootSizes(testPage)).at(-1) ?? 0) / container - 0.2))
        .toBeLessThan(0.01);
    }

    await session.sidebarTaskItem("Notes Environment").click();
    await expect(testPage.getByTestId("e2e-notes-panel")).toBeVisible({ timeout: 15_000 });
    expect(await panelPlacement(testPage, PROMPT_ID)).toBeNull();
    expect((await panelPlacement(testPage, NOTES_ID))?.groupPanelIds).toEqual([NOTES_ID]);

    await session.sidebarTaskItem("Prompt Default").click();
    await expect.poll(() => panelPlacement(testPage, PROMPT_ID)).not.toBeNull();
    const [, rightBefore] = await rootSizes(testPage);
    await dragRootSash(testPage, "last", 1, rightBefore + 80);
    await expect.poll(() => hasManualRightWidth(testPage)).toBe(true);
    const manualWidth = (await rootSizes(testPage)).at(-1) ?? 0;
    for (const action of ["hide", "maximize"]) {
      if (action === "hide") await testPage.getByTestId("task-right-panels-toggle").click();
      else await maximizeAgent.click();
      await expect(testPage.locator(".dv-tab", { hasText: PROMPT_TITLE })).toHaveCount(0);
      await testPage.setViewportSize({ width: 1900, height: 900 });
      if (action === "hide") await testPage.getByTestId("task-right-panels-toggle").click();
      else await maximizeAgent.click();
      await expect
        .poll(async () => Math.abs(((await rootSizes(testPage)).at(-1) ?? 0) - manualWidth))
        .toBeLessThan(3);
      await testPage.setViewportSize({ width: 1600, height: 900 });
    }
    void freshEnvironment;
  });
});
