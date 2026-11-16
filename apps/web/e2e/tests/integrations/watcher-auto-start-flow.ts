import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { watchWs } from "../../helpers/causal-waits";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

type WatcherAutoStartFixture = {
  page: Page;
  api: ApiClient;
  seedData: SeedData;
  mobile: boolean;
};

async function createIssueWatch(
  api: ApiClient,
  workspaceId: string,
  workflowId: string,
  workflowStepId: string,
  agentProfileId: string,
) {
  const instance = await api.createSentryInstance({
    workspaceId,
    name: `Watcher auto-start ${workspaceId}`,
    url: "https://sentry.watcher.test",
    secret: "sntrys_watcher_auto_start_fixture",
  });
  await api.mockSentrySetAuthHealth({ instanceId: instance.id, ok: true });
  await api.mockSentrySetOrganizations(instance.id, [
    { id: "watch-org", slug: "watch-org", name: "Watcher org" },
  ]);
  await api.mockSentrySetProjects(instance.id, [
    {
      id: "watch-project",
      slug: "watch-project",
      name: "Watcher project",
      orgSlug: "watch-org",
    },
  ]);
  const watch = await api.createSentryIssueWatch({
    workspaceId,
    sentryInstanceId: instance.id,
    workflowId,
    workflowStepId,
    agentProfileId,
    orgSlug: "watch-org",
    projectSlugs: ["watch-project"],
  });
  return { instance, watch };
}

async function seedIssue(api: ApiClient, instanceId: string, title: string, issueId: string) {
  const endpoint = `/api/v1/sentry/mock/issues?instanceId=${encodeURIComponent(instanceId)}`;
  const response = await api.rawRequest("POST", endpoint, {
    issues: [
      {
        id: issueId,
        shortId: `WATCH-${issueId.slice(0, 6).toUpperCase()}`,
        title,
        permalink: `https://sentry.watcher.test/organizations/watch-org/issues/${issueId}/`,
        projectSlug: "watch-project",
        level: "error",
        status: "unresolved",
      },
    ],
  });
  if (!response.ok) {
    throw new Error(
      `Could not seed the Sentry issue (${response.status}): ${await response.text()}`,
    );
  }
}

async function triggerWatch(api: ApiClient, watchId: string, workspaceId: string) {
  const endpoint = `/api/v1/sentry/watches/issue/${encodeURIComponent(watchId)}/trigger?workspace_id=${encodeURIComponent(workspaceId)}`;
  const response = await api.rawRequest("POST", endpoint);
  if (!response.ok) {
    throw new Error(
      `Could not trigger the Sentry watch (${response.status}): ${await response.text()}`,
    );
  }
  return (await response.json()) as { published: number };
}

async function createWorkspaceWorkflow(api: ApiClient, name: string) {
  const workspace = await api.createWorkspace(name);
  const workflow = await api.createWorkflow(workspace.id, name);
  const step = await api.createWorkflowStep(workflow.id, `${name} step`, 0, {
    is_start_step: true,
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const { executors } = await api.listExecutors();
  const localExecutor = executors.find((candidate) => candidate.type === "local");
  expect(localExecutor, "local executor is available to workspace defaults").toBeDefined();
  await api.updateWorkspace(workspace.id, { default_executor_id: localExecutor!.id });
  return { workspace, workflow, step, localExecutor: localExecutor! };
}

async function openWorkspaceBoard(
  page: Page,
  workspaceId: string,
  workflowId: string,
  mobile: boolean,
) {
  await page.goto(
    `/?workspaceId=${encodeURIComponent(workspaceId)}&workflowId=${encodeURIComponent(workflowId)}`,
  );
  await expect(page.getByTestId("kanban-board")).toBeVisible();
  if (mobile) await expect(page.getByTestId("mobile-kanban-layout")).toBeVisible();
}

function taskCard(page: Page, taskId: string, stepId: string, mobile: boolean) {
  if (mobile) {
    return page.getByTestId(`kanban-column-${stepId}`).getByTestId(`task-card-${taskId}`);
  }
  return new KanbanPage(page).taskCard(taskId);
}

async function deleteCreatedTask(api: ApiClient, taskId: string) {
  const { sessions } = await api.listTaskSessions(taskId);
  for (const session of sessions) {
    if (session.state === "STARTING" || session.state === "RUNNING") {
      await api.stopSession({ session_id: session.id, force: true }).catch(() => undefined);
    }
  }
  await api.deleteTask(taskId).catch(() => undefined);
}

export async function exerciseWatcherAutoStartFlow({
  page,
  api,
  seedData,
  mobile,
}: WatcherAutoStartFixture) {
  const name = `Watcher auto-start ${Date.now()}`;
  const createdTaskIds: string[] = [];
  let workspaceId: string | undefined;

  try {
    const { workspace, workflow, step, localExecutor } = await createWorkspaceWorkflow(api, name);
    workspaceId = workspace.id;
    await api.updateWorkspace(workspace.id, {
      default_executor_id: localExecutor.id,
      default_agent_profile_id: seedData.agentProfileId,
    });
    const { instance, watch } = await createIssueWatch(api, workspace.id, workflow.id, step.id, "");
    const title = `${name} task`;
    const issueId = randomUUID();
    await seedIssue(api, instance.id, title, issueId);

    const { published } = await triggerWatch(api, watch.id, workspace.id);
    expect(published).toBe(1);

    let taskId = "";
    await expect
      .poll(async () => {
        const { tasks } = await api.listTasks(workspace.id);
        const task = tasks.find((candidate) => candidate.title.includes(title));
        if (task) taskId = task.id;
        return task?.id ?? null;
      })
      .not.toBeNull();
    createdTaskIds.push(taskId);

    await expect
      .poll(async () => {
        const { sessions } = await api.listTaskSessions(taskId);
        return sessions.length === 1 &&
          sessions[0].agent_profile_id === seedData.agentProfileId &&
          (sessions[0].state === "STARTING" || sessions[0].state === "RUNNING")
          ? sessions[0]
          : null;
      })
      .not.toBeNull();
    const { sessions } = await api.listTaskSessions(taskId);
    expect(sessions).toHaveLength(1);
    expect(sessions[0].agent_profile_id).toBe(seedData.agentProfileId);
    expect(sessions[0].executor_id).toBe(localExecutor.id);
    expect(sessions[0].executor_profile_id ?? "").toBe("");

    // The backend session assertion above happens before the page opens, so no task route or
    // session.ensure call can hide a watcher that failed to start on its own.
    await openWorkspaceBoard(page, workspace.id, workflow.id, mobile);
    const card = taskCard(page, taskId, step.id, mobile);
    await expect(card).toBeVisible();
    await expect(card.getByTestId("task-state-auto-start-failed")).toHaveCount(0);
  } finally {
    for (const taskId of createdTaskIds) await deleteCreatedTask(api, taskId);
    if (workspaceId) {
      await api.deleteAllSentryInstances(workspaceId);
      await api.deleteWorkspace(workspaceId, name);
    }
  }
}

export async function exerciseWatcherAutoStartFailureFlow({
  page,
  api,
  mobile,
}: WatcherAutoStartFixture) {
  const name = `Watcher auto-start failure ${Date.now()}`;
  let workspaceId: string | undefined;
  let taskId: string | undefined;
  let workflowStepId: string | undefined;
  const ws = watchWs(page);

  try {
    const { workspace, workflow, step } = await createWorkspaceWorkflow(api, name);
    workspaceId = workspace.id;
    workflowStepId = step.id;
    const { instance, watch } = await createIssueWatch(api, workspace.id, workflow.id, step.id, "");
    const title = `${name} task`;
    const issueId = randomUUID();
    await seedIssue(api, instance.id, title, issueId);
    await openWorkspaceBoard(page, workspace.id, workflow.id, mobile);

    const markerPublished = ws.waitForEvent("task.updated", {
      where: (payload) => payload.auto_start_failed === true,
    });
    const { published } = await triggerWatch(api, watch.id, workspace.id);
    expect(published).toBe(1);
    const marker = await markerPublished;
    taskId = String(marker.payload.task_id ?? "");
    expect(taskId, "failure notification includes the created task ID").not.toBe("");

    await expect
      .poll(async () => (await api.getTask(taskId!)).metadata?.auto_start_failed)
      .toBe(true);
    const { sessions } = await api.listTaskSessions(taskId);
    expect(sessions).toHaveLength(0);

    const card = taskCard(page, taskId, workflowStepId!, mobile);
    await expect(card).toBeVisible();
    await expect(card.getByTestId("task-state-auto-start-failed")).toBeVisible();
    await page.reload();
    await expect(page.getByTestId("kanban-board")).toBeVisible();
    await expect(
      taskCard(page, taskId, workflowStepId!, mobile).getByTestId("task-state-auto-start-failed"),
    ).toBeVisible();
  } finally {
    if (taskId) await deleteCreatedTask(api, taskId);
    if (workspaceId) {
      await api.deleteAllSentryInstances(workspaceId);
      await api.deleteWorkspace(workspaceId, name);
    }
  }
}
