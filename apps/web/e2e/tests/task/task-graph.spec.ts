import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  assertNoDocumentHorizontalOverflow,
  expectLocatorContainedBy,
} from "../../helpers/layout-assertions";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { openConversationUsageTask } from "../chat/conversation-usage-helpers";

// Task relationship graph (docs/specs/tasks/requirements/task-relationship-graph.md).
// Topology: outside → a → b → c, with sibling d unordered, all children of parent.

type Seed = { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string };

async function seedGraph(apiClient: ApiClient, seed: Seed) {
  const base = {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  };
  const parent = await apiClient.createTask(seed.workspaceId, "Graph parent", base);
  const outside = await apiClient.createTask(seed.workspaceId, "Graph outside", base);
  const child = (title: string, blockedBy?: string) =>
    apiClient.createTask(seed.workspaceId, title, {
      ...base,
      parent_id: parent.id,
      ...(blockedBy ? { blocked_by: [blockedBy], start_when_unblocked: false } : {}),
    });
  const a = await child("Graph A", outside.id);
  const b = await child("Graph B", a.id);
  const c = await child("Graph C", b.id);
  const d = await child("Graph D");
  return { parent, outside, a, b, c, d };
}

const node = (page: import("@playwright/test").Page, id: string) =>
  page.getByTestId(`task-graph-node-${id}`);

async function expectRenderedTarget(control: Locator, size: number, label: string) {
  const box = await control.boundingBox();
  expect(box, `${label}: control has geometry`).not.toBeNull();
  if (!box) return;
  expect(Math.abs(box.width - size), `${label}: width`).toBeLessThanOrEqual(1);
  expect(Math.abs(box.height - size), `${label}: height`).toBeLessThanOrEqual(1);
}

test.describe("Task graph", () => {
  test("opens as a workbench tab, shows order, follows live state and navigates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const g = await seedGraph(apiClient, seedData);
    await testPage.goto(`/t/${g.b.id}`);

    const bar = testPage.getByTestId("chat-status-bar");
    const controls = bar.getByTestId("chat-status-bar-right-controls");
    const graphButton = bar.getByTestId("task-graph-button");
    await expect(graphButton).toHaveCount(1);
    await expect(graphButton).toHaveAccessibleName("Task graph");
    await expect(graphButton).toBeVisible();
    await expectLocatorContainedBy(graphButton, controls, "desktop graph action");
    await expectRenderedTarget(graphButton, 24, "desktop graph action");

    await graphButton.click();
    const panel = testPage.getByTestId("task-graph-panel");
    await expect(panel).toBeVisible();
    await expect(testPage).not.toHaveURL(/panel=task-graph/);
    await expect(panel.getByTestId("task-graph-root")).toHaveText(g.parent.title);
    await expect(panel.getByTestId("task-graph-mode-subtasks")).toHaveAttribute(
      "aria-checked",
      "true",
    );

    await expect(node(testPage, g.outside.id)).toHaveAccessibleName(/step 1$/);
    await expect(node(testPage, g.a.id)).toHaveAccessibleName(/^Graph A, .*, step 2$/);
    await expect(node(testPage, g.b.id)).toHaveAccessibleName(/step 3$/);
    await expect(node(testPage, g.c.id)).toHaveAccessibleName(/step 4$/);
    await expect(node(testPage, g.d.id)).toHaveAccessibleName(/step 1$/);
    await expect(panel.getByText("Outside this task")).toBeVisible();
    await expect(panel.getByText("This task", { exact: true })).toBeVisible();
    await expect(panel.getByTestId("task-graph-legend")).toContainText("Waiting");

    // Hovering A emphasizes its chain and dims the unrelated sibling.
    await node(testPage, g.a.id).hover();
    await expect(node(testPage, g.d.id).getByTestId("task-graph-node-card")).toHaveAttribute(
      "data-dimmed",
      "true",
    );

    // A state change reaches the open graph without reopening it.
    const refetch = waitForHttp(testPage, "GET", new RegExp(`/tasks/${g.b.id}/graph$`));
    await apiClient.updateTaskState(g.a.id, "COMPLETED");
    await refetch;
    await expect(node(testPage, g.a.id)).toHaveAccessibleName(/Completed/);

    await panel.getByTestId("task-graph-mode-chain").click();
    await expect(node(testPage, g.d.id)).toHaveCount(0);

    // Activating a node opens that task and keeps the graph tab, now focused on it.
    await node(testPage, g.c.id).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${g.c.id}$`));
    await expect(testPage.getByTestId("task-graph-panel")).toBeVisible();
    await expect(node(testPage, g.c.id).getByTestId("task-graph-node-card")).toHaveAttribute(
      "data-focused",
      "true",
    );
  });

  test("keeps the graph icon after Share across width and pointer changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId, session } = await openConversationUsageTask(
      testPage,
      apiClient,
      seedData,
      "Graph action neighbors",
    );
    await apiClient.createTask(seedData.workspaceId, "Graph action child", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      parent_id: taskId,
    });
    await testPage.reload();
    await session.waitForLoad();

    const bar = session.chatStatusBar();
    const controls = bar.getByTestId("chat-status-bar-right-controls");
    const graph = controls.getByTestId("task-graph-button");
    const share = controls.getByTestId("share-task-button");
    await expect(graph).toHaveCount(1);
    await expect(graph).toHaveAccessibleName("Task graph");
    await expect(share).toBeVisible();
    await expect(controls.getByTestId("conversation-usage-trigger")).toBeVisible();

    const followsShare = await controls.evaluate((group) => {
      const shareButton = group.querySelector('[data-testid="share-task-button"]');
      const graphButton = group.querySelector('[data-testid="task-graph-button"]');
      return Boolean(
        shareButton &&
        graphButton &&
        shareButton.compareDocumentPosition(graphButton) & Node.DOCUMENT_POSITION_FOLLOWING,
      );
    });
    expect(followsShare).toBe(true);
    await expectLocatorContainedBy(graph, controls, "desktop graph action");
    await expectRenderedTarget(graph, 24, "desktop graph action");

    await testPage.setViewportSize({ width: 390, height: 844 });
    await expectRenderedTarget(graph, 44, "narrow fine-pointer graph action");
    await expectLocatorContainedBy(graph, controls, "narrow fine-pointer graph action");
    await assertNoDocumentHorizontalOverflow(testPage, "narrow graph action");
    await testPage.setViewportSize({ width: 767, height: 844 });
    await expectRenderedTarget(graph, 44, "phone breakpoint graph action");
    await testPage.setViewportSize({ width: 768, height: 900 });
    await expectRenderedTarget(graph, 24, "breakpoint graph action");
    await expectLocatorContainedBy(graph, controls, "breakpoint graph action");

    await graph.click();
    await expect(testPage.getByTestId("task-graph-panel")).toBeVisible();
  });

  test("an open graph tab is kept in the task's saved layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Layouts persist per task environment, so the task needs a started session.
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Graph layout parent",
      seedData.agentProfileId,
      {
        description: 'e2e:message("ready")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.createTask(seedData.workspaceId, "Graph layout child", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      parent_id: parent.id,
    });
    await testPage.goto(`/t/${parent.id}?panel=task-graph`);
    await expect(testPage.getByTestId("task-graph-panel")).toBeVisible();
    await expect(testPage).not.toHaveURL(/panel=task-graph/);
    await testPage.evaluate(() =>
      (window as Window & { __persistDockviewLayout__?: () => void }).__persistDockviewLayout__?.(),
    );
    await testPage.reload();
    await expect(testPage.getByTestId("task-graph-panel")).toBeVisible();
  });

  test("Show in graph opens the board Graph view focused on that task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const g = await seedGraph(apiClient, seedData);
    await testPage.emulateMedia({ colorScheme: "dark" });
    await testPage.goto(`/t/${g.a.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const row = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: g.c.title });
    await row.click({ button: "right" });
    await testPage.getByTestId("task-context-show-in-graph").click();
    await expect(testPage).toHaveURL(new RegExp(`graphFocus=${g.c.id}`));
    const board = testPage.getByTestId("board-graph");
    await expect(board).toBeVisible();
    await expect(board.locator(".react-flow.dark")).toHaveCount(1);
    await expect(testPage.getByTestId("view-toggle-graph")).toHaveAttribute("data-state", "on");
    await expect(node(testPage, g.c.id).getByTestId("task-graph-node-card")).toHaveAttribute(
      "data-focused",
      "true",
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await testPage.getByTestId("view-toggle-kanban").click();
    await kanban.taskCard(g.d.id).click({ button: "right" });
    await testPage.getByTestId("task-context-show-in-graph").click();
    await expect(testPage).toHaveURL(new RegExp(`graphFocus=${g.d.id}`));
    await expect(node(testPage, g.d.id).getByTestId("task-graph-node-card")).toHaveAttribute(
      "data-focused",
      "true",
    );
  });

  test("the board Graph view frames subtasks by parent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const g = await seedGraph(apiClient, seedData);
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await testPage.getByTestId("view-toggle-graph").click();
    const board = testPage.getByTestId("board-graph");
    await expect(board).toBeVisible();
    await expect(
      board.getByTestId("task-graph-frame").filter({ hasText: g.parent.title }),
    ).toHaveCount(1);
    await expect(node(testPage, g.a.id)).toBeVisible();
    await expect(node(testPage, g.outside.id)).toBeVisible();

    // Parallel steps: a and d share no ordering, so they band together.
    await board.getByTestId("task-graph-parallel-toggle").click();
    await expect(
      board.getByTestId("task-graph-frame").filter({ hasText: "can run in parallel" }),
    ).not.toHaveCount(0);
    // The re-laid-out cards are re-fitted into view.
    await expect(node(testPage, g.a.id)).toBeInViewport();
    await expect(
      board.getByTestId("task-graph-frame").filter({ hasText: g.parent.title }),
    ).toHaveCount(0);
    await board.getByTestId("task-graph-parallel-toggle").click();

    // A click selects; Open goes into the selected task.
    await node(testPage, g.b.id).click();
    await expect(board.getByTestId("board-graph-selection")).toContainText(g.b.title);
    await expect(testPage).not.toHaveURL(new RegExp(`/t/${g.b.id}`));
    await board.getByTestId("board-graph-open").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${g.b.id}`));

    // Double-clicking a node opens it directly.
    await kanban.goto(seedData.workflowId);
    await node(testPage, g.a.id).dblclick();
    await expect(testPage).toHaveURL(new RegExp(`/t/${g.a.id}`));
  });

  test("the dependency chip opens the graph tab", async ({ testPage, apiClient, seedData }) => {
    const g = await seedGraph(apiClient, seedData);
    await testPage.goto(`/t/${g.b.id}`);
    await testPage.getByTestId("task-dependency-chip").first().click();
    await testPage.getByTestId("task-dependency-view-graph").click();
    await expect(testPage.getByTestId("task-graph-panel")).toBeVisible();
  });

  test("hides the entry point for a task without relations", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const lonely = await apiClient.createTask(seedData.workspaceId, "Graph lonely", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const subtaskCount = waitForHttp(
      testPage,
      "GET",
      new RegExp(`/tasks/${lonely.id}/subtask-count$`),
    );
    await testPage.goto(`/t/${lonely.id}`);
    await subtaskCount;
    await expect(testPage.getByTestId("task-graph-button")).toHaveCount(0);
  });
});
