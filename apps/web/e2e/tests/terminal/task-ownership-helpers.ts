import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { snapshotPersistedLayouts } from "../../helpers/dockview-persistence";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const LAYOUT_KEY_PREFIX = "kandev.dockview.env-layout-v3.";

export type TerminalRow = {
  id?: string;
  terminal_id?: string;
  kind?: string;
  seq?: number;
  display_name?: string;
  custom_name?: string | null;
  state?: string;
  pty_status?: string;
};

export type TaskWithSession = {
  id: string;
  title: string;
  sessionId: string;
};

export async function createTaskWithSession(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<TaskWithSession> {
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
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sessionId = sessions[0]?.id ?? task.session_id;
  if (!sessionId) throw new Error(`${title} did not expose a session id`);
  return { id: task.id, title, sessionId };
}

/** Child task that inherits the parent's workspace and execution environment. */
export async function createInheritedTask(
  apiClient: ApiClient,
  seedData: SeedData,
  parent: TaskWithSession,
  title: string,
): Promise<TaskWithSession> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
      workspace_mode: "inherit_parent",
    },
  );
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.length > 0;
      },
      { timeout: 45_000, message: `Waiting for ${title} session` },
    )
    .toBe(true);
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sessionId = sessions[0]?.id ?? task.session_id;
  if (!sessionId) throw new Error(`${title} did not expose a session id`);
  return { id: task.id, title, sessionId };
}

export async function taskEnvironmentID(apiClient: ApiClient, taskID: string): Promise<string> {
  const { sessions } = await apiClient.listTaskSessions(taskID);
  const environmentID = sessions[0]?.task_environment_id;
  expect(environmentID, `task ${taskID} should have an environment`).toBeTruthy();
  return environmentID as string;
}

export async function expectSharedEnvironment(
  apiClient: ApiClient,
  left: TaskWithSession,
  right: TaskWithSession,
): Promise<string> {
  const leftEnvironment = await taskEnvironmentID(apiClient, left.id);
  const rightEnvironment = await taskEnvironmentID(apiClient, right.id);
  expect(rightEnvironment, "both tasks must share one execution environment").toBe(leftEnvironment);
  return leftEnvironment;
}

export async function listOrdinaryTerminals(
  apiClient: ApiClient,
  taskID: string,
  environmentID: string,
): Promise<TerminalRow[]> {
  const response = await apiClient.wsRequest<{ shells: TerminalRow[] }>("user_shell.list", {
    task_id: taskID,
    task_environment_id: environmentID,
    include_parked: true,
  });
  return (response?.shells ?? []).filter((row) => row.kind === "ordinary");
}

export async function createOrdinaryTerminal(
  apiClient: ApiClient,
  taskID: string,
  environmentID: string,
): Promise<TerminalRow> {
  const created = await apiClient.wsRequest<TerminalRow>("user_shell.create", {
    task_id: taskID,
    task_environment_id: environmentID,
  });
  return { ...created, id: created.id ?? created.terminal_id };
}

/** Navigates to a task and brings its dockview Terminal tab to the front. */
export async function openTaskTerminal(page: Page, taskID: string): Promise<SessionPage> {
  await page.goto(`/t/${taskID}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.clickTab("Terminal");
  await session.expectTerminalConnected();
  return session;
}

/** Types a command into the visible dockview terminal and waits for its output. */
export async function runTerminalCommand(
  session: SessionPage,
  command: string,
  expected: string,
): Promise<void> {
  await session.typeInTerminal(command);
  await session.expectTerminalHasText(expected);
}

export async function expectTerminalMissing(session: SessionPage, text: string): Promise<void> {
  await expect
    .poll(async () => (await session.readTerminalBuffer()).includes(text), {
      timeout: 5_000,
      message: `Expected the visible terminal not to contain "${text}"`,
    })
    .toBe(false);
}

/** Terminal id currently stamped on the restored default panel. */
export async function defaultPanelTerminalID(page: Page): Promise<string | null> {
  return page.evaluate(() => {
    type Panel = { params?: Record<string, unknown> };
    const api = (
      window as unknown as { __dockviewApi__?: { getPanel: (id: string) => Panel | null } }
    ).__dockviewApi__;
    const terminalID = api?.getPanel("terminal-default")?.params?.terminalId;
    return typeof terminalID === "string" ? terminalID : null;
  });
}

/**
 * Rewrites the persisted default panel to point at a terminal from another
 * task, reproducing a restored contaminated layout. The caller reloads after.
 */
export async function contaminateDefaultTerminalPanel(
  page: Page,
  environmentID: string,
  terminalID: string,
): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(
          ({ prefix, envID }) =>
            window.sessionStorage.getItem(`${prefix}${envID}`)?.includes('"terminal-default"') ??
            false,
          { prefix: LAYOUT_KEY_PREFIX, envID: environmentID },
        ),
      { timeout: 10_000, message: "the default terminal panel was never persisted" },
    )
    .toBe(true);
  const before = await snapshotPersistedLayouts(page);
  await page.evaluate(
    ({ prefix, envID, foreignTerminalID }) => {
      const key = `${prefix}${envID}`;
      const raw = window.sessionStorage.getItem(key);
      if (!raw) throw new Error(`no persisted layout for environment ${envID}`);
      const layout = JSON.parse(raw) as {
        panels?: Record<string, { params?: Record<string, unknown> }>;
      };
      const panel = layout.panels?.["terminal-default"];
      if (!panel) throw new Error("persisted layout has no terminal-default panel");
      panel.params = { ...(panel.params ?? {}), terminalId: foreignTerminalID };
      window.sessionStorage.setItem(key, JSON.stringify(layout));
    },
    { prefix: LAYOUT_KEY_PREFIX, envID: environmentID, foreignTerminalID: terminalID },
  );
  // The app saves layouts on a debounce. Patch immediately before the caller's
  // reload so a later save cannot overwrite the contaminated snapshot.
  const patched = await snapshotPersistedLayouts(page);
  expect(patched).not.toBe(before);
}

/** Opens the mobile terminal picker and returns the visible row ids. */
export async function mobileTerminalRowIDs(page: Page): Promise<string[]> {
  await page.getByTestId("mobile-terminals-pill").tap();
  const dialog = page.getByRole("dialog", { name: "Terminals", exact: true });
  await expect(dialog).toBeVisible();
  const rows = dialog.locator('[data-testid^="mobile-terminal-row-"]');
  await expect(rows.first()).toBeVisible();
  const testIDs = await rows.evaluateAll((elements) =>
    elements.map((element) => element.getAttribute("data-testid") ?? ""),
  );
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  return testIDs
    .filter((testID) => testID.startsWith("mobile-terminal-row-"))
    .map((testID) => testID.replace("mobile-terminal-row-", ""));
}
