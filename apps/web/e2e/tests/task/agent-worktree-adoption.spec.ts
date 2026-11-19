import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  agentWorktreeGroup,
  agentWorktreePrompt,
  AGENT_WORKTREE_BRANCH,
  AGENT_WORKTREE_FILE,
  expectAgentWorktreeAttached,
  expectAgentWorktreeGroup,
  removeAgentWorktreeRoot,
  waitForAgentWorktreeStatus,
  waitForAgentWorktreeTaskUpdate,
} from "../../helpers/agent-worktree-adoption";
import { watchWs } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

type MCPChild = Awaited<ReturnType<typeof inspectMCPChild>>;
type TaskEnvironment = NonNullable<Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>>;
type EnvironmentRepository = NonNullable<TaskEnvironment["repos"]>[number];

async function waitForMCPChildChanges(page: Page) {
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await session.clickTab("Changes");
}

async function expectMCPChildChanges(page: Page, shared: boolean) {
  if (shared) {
    const group = await expectAgentWorktreeGroup(page);
    await expect(group.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible();
    return;
  }
  await expect(agentWorktreeGroup(page)).toHaveCount(0);
  await expect(page.getByText(AGENT_WORKTREE_FILE, { exact: true })).toHaveCount(0);
}

async function expectMCPChildWorkspaceInChanges(page: Page, taskId: string, shared: boolean) {
  await page.goto(`/t/${taskId}`);
  await waitForMCPChildChanges(page);
  await expectMCPChildChanges(page, shared);

  await page.reload();
  await waitForMCPChildChanges(page);
  await expectMCPChildChanges(page, shared);
}

async function waitForMCPChildren(
  apiClient: ApiClient,
  workspaceId: string,
  titles: Record<string, string>,
): Promise<Map<string, string>> {
  const childIDs = new Map<string, string>();
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(workspaceId);
        for (const title of Object.values(titles)) {
          const child = tasks.find((task) => task.title === title);
          if (child) childIDs.set(title, child.id);
        }
        return childIDs.size;
      },
      { timeout: 60_000, message: "all three MCP children should be created" },
    )
    .toBe(Object.keys(titles).length);
  return childIDs;
}

async function inspectMCPChild(apiClient: ApiClient, parentId: string, id: string, key: string) {
  await expect
    .poll(async () => (await apiClient.getTask(id)).primary_session_id ?? null, {
      timeout: 60_000,
      message: `${key} child primary session should be published`,
    })
    .toBeTruthy();
  const task = await apiClient.getTask(id);
  expect(task.parent_id).toBe(parentId);
  if (!task.primary_session_id) throw new Error(`${key} child primary session was not published`);
  await waitForSessionDone(apiClient, id, task.primary_session_id, `${key} child`, 180_000);
  if (key !== "shared") {
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(id))?.status ?? null, {
        timeout: 90_000,
        message: `${key} child workspace should become ready`,
      })
      .toBe("ready");
  }
  const { sessions } = await apiClient.listTaskSessions(id);
  const session = sessions.find((item) => item.id === task.primary_session_id);
  expect(session, `${key} child session should be listed`).toBeDefined();
  if (!session) throw new Error(`${key} child session was not listed`);
  expect(session.state).not.toBe("FAILED");
  return {
    key,
    task,
    environment: await apiClient.getTaskEnvironment(id),
    session,
  };
}

function expectMCPWorkspaceOutcomes(
  children: MCPChild[],
  parentEnvironment: TaskEnvironment,
  adopted: EnvironmentRepository,
) {
  const freshChildren = children.filter((child) => child.key !== "shared");
  const sharedChild = children.find((child) => child.key === "shared");
  expect(sharedChild).toBeDefined();
  if (!sharedChild) throw new Error("shared MCP child was not found");

  expect(freshChildren).toHaveLength(2);
  expect(new Set(freshChildren.map((child) => child.environment?.id)).size).toBe(2);
  expect(
    new Set(freshChildren.map((child) => child.environment?.id)).has(parentEnvironment.id),
  ).toBe(false);
  const freshRepositories = freshChildren.map((child) => child.environment?.repos?.[0]);
  expect(freshRepositories.every((repository) => Boolean(repository?.worktree_id))).toBe(true);
  expect(new Set(freshRepositories.map((repository) => repository?.worktree_id)).size).toBe(2);
  expect(new Set(freshRepositories.map((repository) => repository?.worktree_path)).size).toBe(2);
  expect(new Set(freshRepositories.map((repository) => repository?.worktree_branch)).size).toBe(2);
  for (const child of freshChildren) {
    expect(child.task.metadata?.workspace).toMatchObject({ mode: "new_workspace" });
    expect(child.task.repositories).toHaveLength(1);
    expect(child.task.repositories?.[0]?.checkout_branch).toBeFalsy();
  }

  expect(sharedChild.task.metadata?.workspace).toMatchObject({ mode: "inherit_parent" });
  expect(sharedChild.environment).toBeNull();
  expect(sharedChild.session.task_environment_id).toBe(parentEnvironment.id);
  expect(
    (sharedChild.session.worktrees ?? [])
      .map((worktree) => worktree.worktree_id)
      .filter((id): id is string => Boolean(id))
      .sort(),
  ).toEqual(
    (parentEnvironment.repos ?? [])
      .map((repository) => repository.worktree_id)
      .filter((id): id is string => Boolean(id))
      .sort(),
  );
  const sharedAdopted = sharedChild.session.worktrees?.find(
    (worktree) => worktree.worktree_id === adopted.worktree_id,
  );
  expect(sharedAdopted).toBeDefined();
  if (sharedAdopted?.worktree_path) {
    expect(sharedAdopted.worktree_path).toBe(adopted.worktree_path);
  }
  expect(
    execFileSync(
      "git",
      ["-C", adopted.worktree_path!, "status", "--porcelain", "--", AGENT_WORKTREE_FILE],
      { encoding: "utf8" },
    ),
  ).toContain(AGENT_WORKTREE_FILE);
}

test.describe("Agent worktree adoption", () => {
  test.setTimeout(240_000);

  test("detects, removes, and re-attaches an agent-created worktree", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Agent worktree adoption",
      seedData.agentProfileId,
      {
        description: agentWorktreePrompt(false),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "initial turn", 120_000);

    // Detection runs from the live session poll, so the page has to be open.
    // The status event is the git snapshot that renders the adopted file list.
    const watcher = watchWs(testPage);
    const statusPublished = waitForAgentWorktreeStatus(watcher, task.session_id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await session.clickTab("Changes");
    await expectAgentWorktreeAttached(apiClient, task.id);
    await statusPublished;

    const group = await expectAgentWorktreeGroup(testPage);
    await expect(group.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible();

    await removeAgentWorktreeRoot(testPage);

    const reattachedTask = waitForAgentWorktreeTaskUpdate(watcher, task.id);
    const reattachedStatus = waitForAgentWorktreeStatus(watcher, task.session_id);
    await apiClient.addUserMessage(task.id, task.session_id, agentWorktreePrompt(true));
    await waitForSessionDone(apiClient, task.id, task.session_id, "reported turn", 120_000);
    await reattachedTask;
    await reattachedStatus;

    const reattached = await expectAgentWorktreeGroup(testPage);
    await expect(reattached.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible();
  });

  test("MCP children use fresh or shared worktrees", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(360_000);

    const suffix = Date.now();
    const titles = {
      freshOne: `MCP fresh checkout one ${suffix}`,
      freshTwo: `MCP fresh checkout two ${suffix}`,
      shared: `MCP shared checkout ${suffix}`,
    };
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `MCP checkout parent ${suffix}`,
      seedData.agentProfileId,
      {
        description: agentWorktreePrompt(false),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!parent.session_id) throw new Error("parent task creation did not return a session id");

    const watcher = watchWs(testPage);
    const parentStatus = waitForAgentWorktreeStatus(watcher, parent.session_id);
    await testPage.goto(`/t/${parent.id}`);
    const parentSession = new SessionPage(testPage);
    await parentSession.waitForLoad();
    await waitForSessionDone(
      apiClient,
      parent.id,
      parent.session_id,
      "create adopted checkout",
      120_000,
    );
    await expectAgentWorktreeAttached(apiClient, parent.id);
    await parentStatus;

    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(parent.id))?.status ?? null, {
        timeout: 60_000,
      })
      .toBe("ready");
    const parentEnvironment = await apiClient.getTaskEnvironment(parent.id);
    if (!parentEnvironment) throw new Error("parent task environment was not created");
    const adopted = parentEnvironment.repos?.find((repository) =>
      repository.worktree_branch?.includes(AGENT_WORKTREE_BRANCH),
    );
    if (!adopted?.worktree_id || !adopted.worktree_path) {
      throw new Error("agent-created worktree was not attached to the parent environment");
    }
    const markerStatus = execFileSync(
      "git",
      ["-C", adopted.worktree_path, "status", "--porcelain", "--", AGENT_WORKTREE_FILE],
      { encoding: "utf8" },
    );
    expect(markerStatus).toContain(AGENT_WORKTREE_FILE);

    const createChild = (title: string, workspaceMode?: string) =>
      `e2e:mcp:kandev:create_task_kandev(${JSON.stringify({
        parent_id: "self",
        title,
        description: "Verify MCP workspace selection.",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        ...(workspaceMode ? { workspace_mode: workspaceMode } : {}),
      })})`;
    const createChildrenScript = [
      createChild(titles.freshOne),
      createChild(titles.freshTwo),
      createChild(titles.shared, "inherit_parent"),
      'e2e:message("Children created.")',
    ].join("\n");
    await apiClient.addUserMessage(parent.id, parent.session_id, createChildrenScript);
    await waitForSessionDone(
      apiClient,
      parent.id,
      parent.session_id,
      "create MCP children",
      120_000,
    );

    const childIDs = await waitForMCPChildren(apiClient, seedData.workspaceId, titles);
    const children = await Promise.all(
      Object.entries(titles).map(async ([key, title]) => {
        const id = childIDs.get(title);
        if (!id) throw new Error(`MCP child ${title} was not created`);
        return inspectMCPChild(apiClient, parent.id, id, key);
      }),
    );
    expectMCPWorkspaceOutcomes(children, parentEnvironment, adopted);
    for (const child of children) {
      await expectMCPChildWorkspaceInChanges(testPage, child.task.id, child.key === "shared");
    }
  });
});
