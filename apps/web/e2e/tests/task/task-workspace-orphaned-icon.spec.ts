import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { watchWs } from "../../helpers/causal-waits";

// Workspace-orphaned indicator: when a parent task with an inherit_parent
// child is archived, the child's materialized workspace is torn down but the
// child itself survives as a live, unstartable row. The backend stamps
// metadata.workspace.orphaned = true (see internal/task/models.WorkspaceOrphaned)
// and both board surfaces — the kanban card and the pipeline graph node —
// show a distinct icon instead of silently rendering like a normal task.
// Seeding the marker through the public task-metadata surface is the
// deterministic stand-in for a real archive-time strand; the icon itself
// must render from that marker alone.
//
// The derivation is a conjunction, not a presence test: metadata.workspace.
// orphaned must be strictly boolean true AND metadata.workspace.mode must
// still be "inherit_parent" (AC-001.9). A task moved off inherit_parent
// (reparent, detach, non-cascade delete) without the marker being retracted
// must not show the icon — the "mode mismatch" fixture below pins that.
//
// The fixture is left in a session-less IN_PROGRESS state on purpose: the
// real shape is a task mid-flight when its parent gets archived (no
// session, task state still IN_PROGRESS/SCHEDULING), which previously made
// shouldShowTaskRunningSpinner report true and short-circuited
// renderTaskStatusIcon straight to the launch spinner before the marker
// ever got a chance to render (fixed in kanban-card-content.tsx's
// foregroundActivity forcing condition).
//
// The marker is seeded via a PATCH (updateTaskMetadata) AFTER creation, never
// through createTask's own `metadata` option: httpCreateTask resolves a
// workspace policy for every new task and MergeMetadataBlock (handoff_service.go)
// unconditionally rewrites metadata.workspace from that policy (defaulting
// mode to "new_workspace" when the caller didn't ask for inherit_parent/
// shared_group), discarding any orphaned/orphaned_* keys passed at create
// time. Production never stamps this marker at creation either — it is
// always written onto an already-existing task (handoff_workspace_orphan.go)
// — so seeding it post-creation via PATCH is the accurate stand-in, not a
// workaround.

const ORPHANED_WORKSPACE_METADATA = {
  workspace: {
    mode: "inherit_parent",
    orphaned: true,
    orphaned_parent_id: "11111111-1111-1111-1111-111111111111",
    orphaned_reason: "parent_archived",
    orphaned_at: "2026-09-05T10:00:00Z",
  },
};

const MODE_MISMATCH_WORKSPACE_METADATA = {
  workspace: {
    mode: "new_workspace",
    orphaned: true,
    orphaned_parent_id: "11111111-1111-1111-1111-111111111111",
    orphaned_reason: "parent_archived",
    orphaned_at: "2026-09-05T10:00:00Z",
  },
};

test.describe("Kanban board — workspace-orphaned icon", () => {
  test("shows the icon only for marked, non-terminal, inherit_parent tasks on both board surfaces", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const orphaned = await apiClient.createTask(
      seedData.workspaceId,
      "Orphaned Workspace Fixture",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await apiClient.updateTaskMetadata(orphaned.id, ORPHANED_WORKSPACE_METADATA);
    // Session-less IN_PROGRESS: the exact shape a real archive-time strand
    // leaves behind (task left mid-flight, no session ever attached).
    await apiClient.updateTaskState(orphaned.id, "IN_PROGRESS");

    const plain = await apiClient.createTask(seedData.workspaceId, "Plain Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(plain.id, "REVIEW");

    // A terminal marked task must keep its done affordance, never the marker.
    const terminal = await apiClient.createTask(seedData.workspaceId, "Terminal Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskMetadata(terminal.id, ORPHANED_WORKSPACE_METADATA);
    await apiClient.updateTaskState(terminal.id, "COMPLETED");

    // AC-001.9: orphaned=true but mode has since moved off inherit_parent
    // (reparent/detach without the marker being retracted) must not render.
    const modeMismatch = await apiClient.createTask(seedData.workspaceId, "Mode Mismatch Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskMetadata(modeMismatch.id, MODE_MISMATCH_WORKSPACE_METADATA);
    await apiClient.updateTaskState(modeMismatch.id, "IN_PROGRESS");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const orphanedCard = kanban.taskCardByTitle("Orphaned Workspace Fixture");
    await expect(orphanedCard).toBeVisible({ timeout: 20_000 });
    await expect(orphanedCard.getByTestId("task-state-workspace-orphaned")).toBeVisible({
      timeout: 20_000,
    });

    const plainCard = kanban.taskCardByTitle("Plain Fixture");
    await expect(plainCard).toBeVisible({ timeout: 20_000 });
    await expect(plainCard.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);

    const terminalCard = kanban.taskCardByTitle("Terminal Fixture");
    await expect(terminalCard).toBeVisible({ timeout: 20_000 });
    await expect(terminalCard.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);

    const modeMismatchCard = kanban.taskCardByTitle("Mode Mismatch Fixture");
    await expect(modeMismatchCard).toBeVisible({ timeout: 20_000 });
    await expect(modeMismatchCard.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);

    // Reload: the marker must survive SSR hydration from the boot payload.
    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(kanban.taskCardByTitle("Orphaned Workspace Fixture")).toBeVisible({
      timeout: 20_000,
    });
    await expect(
      kanban
        .taskCardByTitle("Orphaned Workspace Fixture")
        .getByTestId("task-state-workspace-orphaned"),
    ).toBeVisible({ timeout: 20_000 });
    await expect(
      kanban.taskCardByTitle("Plain Fixture").getByTestId("task-state-workspace-orphaned"),
    ).toHaveCount(0);

    // Pipeline (graph2) surface gates and threads workspaceOrphaned
    // independently of the kanban card — verify it separately rather than
    // treating the card assertion above as covering it too.
    await kanban.switchToPipelineView();
    const orphanedNode = kanban.pipelineTask(orphaned.id);
    await expect(orphanedNode).toBeVisible({ timeout: 20_000 });
    await expect(orphanedNode.getByTestId("task-state-workspace-orphaned")).toBeVisible({
      timeout: 20_000,
    });

    const plainNode = kanban.pipelineTask(plain.id);
    await expect(plainNode).toBeVisible({ timeout: 20_000 });
    await expect(plainNode.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);

    const modeMismatchNode = kanban.pipelineTask(modeMismatch.id);
    await expect(modeMismatchNode).toBeVisible({ timeout: 20_000 });
    await expect(modeMismatchNode.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);
  });

  // Every case above seeds the marker before the page ever loads, so it only
  // exercises the HTTP snapshot / boot-payload producer. This test sets and
  // clears the marker via a live PATCH while the board stays open, so only
  // the WS path (lib/ws/handlers/kanban.ts's two producer sites) can make it
  // pass — the same shape that shipped broken once for auto_start_failed.
  test("shows and hides the icon over a live WS update, without a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTask(seedData.workspaceId, "Live WS Orphaned Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(task.id, "IN_PROGRESS");

    const kanban = new KanbanPage(testPage);
    const ws = watchWs(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Live WS Orphaned Fixture");
    await expect(card).toBeVisible({ timeout: 20_000 });
    await expect(card.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);

    const markerSet = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.workspace_orphaned === true,
    });
    await apiClient.updateTaskMetadata(task.id, ORPHANED_WORKSPACE_METADATA);
    await markerSet;

    await expect(card.getByTestId("task-state-workspace-orphaned")).toBeVisible({
      timeout: 10_000,
    });

    const markerCleared = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.workspace_orphaned === false,
    });
    await apiClient.updateTaskMetadata(task.id, {});
    await markerCleared;

    await expect(card.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);
  });
});
