import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

async function createTaskAndWaitForDone(apiClient: ApiClient, seedData: SeedData, title: string) {
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
      { timeout: 30_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);
  return task;
}

async function navigateToTaskViaKanban(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

type ListItem = {
  id?: string;
  kind?: string;
  seq?: number;
  display_name?: string;
  custom_name?: string | null;
  state?: string;
  pty_status?: string;
};

async function listTerminals(
  apiClient: ApiClient,
  taskID: string,
  envID: string,
  includeParked: boolean,
): Promise<ListItem[]> {
  const resp = await apiClient.wsRequest<{ shells: ListItem[] }>("user_shell.list", {
    task_id: taskID,
    task_environment_id: envID,
    include_parked: includeParked,
  });
  return resp?.shells ?? [];
}

async function createOrdinaryTerminal(
  apiClient: ApiClient,
  taskID: string,
  envID: string,
): Promise<{ terminal_id: string; seq?: number; display_name?: string; kind?: string }> {
  return apiClient.wsRequest("user_shell.create", {
    task_id: taskID,
    task_environment_id: envID,
  });
}

async function renameTerminal(
  apiClient: ApiClient,
  taskID: string,
  terminalID: string,
  name: string | null,
): Promise<void> {
  await apiClient.wsRequest("user_shell.rename", {
    task_id: taskID,
    terminal_id: terminalID,
    custom_name: name,
  });
}

async function parkTerminal(
  apiClient: ApiClient,
  taskID: string,
  terminalID: string,
): Promise<void> {
  await apiClient.wsRequest("user_shell.park", {
    task_id: taskID,
    terminal_id: terminalID,
  });
}

async function resumeTerminal(
  apiClient: ApiClient,
  taskID: string,
  terminalID: string,
): Promise<void> {
  await apiClient.wsRequest("user_shell.resume", {
    task_id: taskID,
    terminal_id: terminalID,
  });
}

async function destroyTerminal(
  apiClient: ApiClient,
  taskID: string,
  terminalID: string,
  envID: string,
): Promise<void> {
  await apiClient.wsRequest("user_shell.destroy", {
    task_id: taskID,
    task_environment_id: envID,
    terminal_id: terminalID,
  });
}

async function getEnvID(apiClient: ApiClient, taskID: string): Promise<string> {
  const { sessions } = await apiClient.listTaskSessions(taskID);
  const envID = sessions[0]?.task_environment_id;
  expect(envID, "task should have an environment id").toBeTruthy();
  return envID as string;
}

test.describe("Terminals — first-class persistent entities", () => {
  test("ordinary terminals get monotonic seqs", async ({ apiClient, seedData }) => {
    test.setTimeout(60_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Monotonic Seq");
    const envID = await getEnvID(apiClient, task.id);

    const t1 = await createOrdinaryTerminal(apiClient, task.id, envID);
    const t2 = await createOrdinaryTerminal(apiClient, task.id, envID);
    const t3 = await createOrdinaryTerminal(apiClient, task.id, envID);

    expect(t1.kind).toBe("ordinary");
    expect(t1.seq).toBe(1);
    expect(t2.seq).toBe(2);
    expect(t3.seq).toBe(3);
  });

  test("rename + park survive a page refresh", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Rename Park Persist");
    const envID = await getEnvID(apiClient, task.id);
    const session = await navigateToTaskViaKanban(testPage, "Rename Park Persist");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Visiting the session page migrates the legacy `shell-default` panel
    // into a DB-backed ordinary terminal (seq=1) — every subsequent
    // create starts from seq=2. Look up the migrated row instead of
    // assuming seq numbering.
    const a = await createOrdinaryTerminal(apiClient, task.id, envID);
    const b = await createOrdinaryTerminal(apiClient, task.id, envID);
    const c = await createOrdinaryTerminal(apiClient, task.id, envID);

    await renameTerminal(apiClient, task.id, b.terminal_id, "build watcher");
    await parkTerminal(apiClient, task.id, c.terminal_id);

    // Reload — DB-backed metadata must survive.
    await testPage.reload();
    await session.waitForLoad();

    const all = await listTerminals(apiClient, task.id, envID, true);
    const ordinary = all.filter((it) => it.kind === "ordinary");
    // 3 API-created + 1 migrated-default
    expect(ordinary).toHaveLength(4);
    const byID = Object.fromEntries(ordinary.map((it) => [it.id, it]));
    expect(byID[a.terminal_id]?.state).toBe("open");
    expect(byID[b.terminal_id]?.custom_name).toBe("build watcher");
    expect(byID[b.terminal_id]?.display_name).toBe("build watcher");
    expect(byID[c.terminal_id]?.state).toBe("parked");

    // The open list should hide the parked terminal.
    const openList = await listTerminals(apiClient, task.id, envID, false);
    const openOrdinary = openList.filter((it) => it.kind === "ordinary");
    expect(openOrdinary.find((it) => it.id === c.terminal_id)).toBeUndefined();
  });

  test("destroy + recreate leaves a stable gap in the seq sequence", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Seq Gap Preservation");
    const envID = await getEnvID(apiClient, task.id);

    const a = await createOrdinaryTerminal(apiClient, task.id, envID);
    const b = await createOrdinaryTerminal(apiClient, task.id, envID);
    const c = await createOrdinaryTerminal(apiClient, task.id, envID);
    expect([a.seq, b.seq, c.seq]).toEqual([1, 2, 3]);

    await destroyTerminal(apiClient, task.id, b.terminal_id, envID);

    const d = await createOrdinaryTerminal(apiClient, task.id, envID);
    expect(d.seq, "deleted #2 should leave a gap — next create should be #4").toBe(4);

    const all = await listTerminals(apiClient, task.id, envID, true);
    const seqs = all
      .filter((it) => it.kind === "ordinary")
      .map((it) => it.seq ?? 0)
      .sort((x, y) => x - y);
    expect(seqs).toEqual([1, 3, 4]);
  });

  test("park + resume keeps the row but toggles state", async ({ apiClient, seedData }) => {
    test.setTimeout(60_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Park Resume Cycle");
    const envID = await getEnvID(apiClient, task.id);

    const t = await createOrdinaryTerminal(apiClient, task.id, envID);

    await parkTerminal(apiClient, task.id, t.terminal_id);
    const openOnly = await listTerminals(apiClient, task.id, envID, false);
    expect(openOnly.find((it) => it.id === t.terminal_id)).toBeUndefined();

    const all = await listTerminals(apiClient, task.id, envID, true);
    expect(all.find((it) => it.id === t.terminal_id)?.state).toBe("parked");

    await resumeTerminal(apiClient, task.id, t.terminal_id);
    const afterResume = await listTerminals(apiClient, task.id, envID, false);
    expect(afterResume.find((it) => it.id === t.terminal_id)?.state).toBe("open");
  });

  test("rename/park guards reject the bottom-panel terminal", async ({ apiClient, seedData }) => {
    test.setTimeout(60_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Bottom Panel Guard");
    await getEnvID(apiClient, task.id);

    await expect(
      renameTerminal(apiClient, task.id, "bottom-panel", "should fail"),
      "rename guard",
    ).rejects.toThrow();
    await expect(parkTerminal(apiClient, task.id, "bottom-panel"), "park guard").rejects.toThrow();
    await expect(parkTerminal(apiClient, task.id, "script-fake"), "script guard").rejects.toThrow();
  });

  test("cross-task mutating ops are rejected", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);
    const taskA = await createTaskAndWaitForDone(apiClient, seedData, "Owner Task A");
    const taskB = await createTaskAndWaitForDone(apiClient, seedData, "Attacker Task B");
    const envA = await getEnvID(apiClient, taskA.id);

    // Create a terminal owned by task A.
    const owned = await createOrdinaryTerminal(apiClient, taskA.id, envA);

    // Task B tries to rename/park/destroy task A's terminal by raw id.
    await expect(
      renameTerminal(apiClient, taskB.id, owned.terminal_id, "stolen"),
      "cross-task rename should reject",
    ).rejects.toThrow();
    await expect(
      parkTerminal(apiClient, taskB.id, owned.terminal_id),
      "cross-task park should reject",
    ).rejects.toThrow();
    await expect(
      destroyTerminal(apiClient, taskB.id, owned.terminal_id, envA),
      "cross-task destroy should reject",
    ).rejects.toThrow();

    // Terminal is still owned by task A, unchanged.
    const list = await listTerminals(apiClient, taskA.id, envA, true);
    const found = list.find((it) => it.id === owned.terminal_id);
    expect(found, "terminal should still exist under task A").toBeDefined();
    expect(found?.custom_name ?? null).toBeNull();
    expect(found?.state).toBe("open");
  });
});
