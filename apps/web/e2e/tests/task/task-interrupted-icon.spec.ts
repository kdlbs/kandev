import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Interrupted-task indicator: when Kandev dies while a task's session is
// mid-turn, startup reconciliation marks the task with the `interrupted_at`
// metadata key and the task list shows a warning triangle until the task is
// resumed. Seeding `interrupted_at` through the public task-metadata surface
// is the deterministic stand-in for a real crash; the icon itself must render
// from that marker alone.

test.describe("Task list — interrupted-task icon", () => {
  test("shows the warning triangle only for marked tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Anchor task with an agent so we can open a session page and view the
    // sidebar in its normal SSR-hydrated context.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Anchor Session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // The post-restart shape: a marked task reconciled to REVIEW with a
    // WAITING_FOR_INPUT session.
    const interrupted = await apiClient.createTask(seedData.workspaceId, "Interrupted Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      metadata: { interrupted_at: "2026-08-02T10:00:00Z" },
    });
    await apiClient.updateTaskState(interrupted.id, "REVIEW");

    const plain = await apiClient.createTask(seedData.workspaceId, "Plain Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(plain.id, "REVIEW");

    // A terminal marked task must keep its done affordance, never the red icon.
    const terminal = await apiClient.createTask(seedData.workspaceId, "Terminal Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      metadata: { interrupted_at: "2026-08-02T10:00:00Z" },
    });
    await apiClient.updateTaskState(terminal.id, "COMPLETED");

    const kanban = new KanbanPage(testPage);
    const workflowSnapshot = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        response.url().includes(`/api/v1/workflows/${seedData.workflowId}/snapshot`) &&
        response.ok(),
    );
    await kanban.goto();
    await workflowSnapshot;
    const interruptedCard = kanban.taskCard(interrupted.id);
    await expect(interruptedCard).toBeVisible();
    const cardIcon = interruptedCard.getByTestId("task-state-interrupted");
    await expect(cardIcon).toBeVisible();
    await expect(cardIcon).toHaveClass(/tabler-icon-alert-triangle/);
    await expect(cardIcon).toHaveClass(/text-yellow-500/);

    const anchorCard = kanban.taskCardByTitle("Anchor Session");
    await expect(anchorCard).toBeVisible();
    await anchorCard.click();
    await expect(testPage).toHaveURL(/\/t\//);

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const interruptedRow = session.sidebarTaskItem("Interrupted Fixture");
    await expect(interruptedRow).toBeVisible();
    await expect(interruptedRow.getByTestId("task-state-interrupted")).toBeVisible();

    const plainRow = session.sidebarTaskItem("Plain Fixture");
    await expect(plainRow).toBeVisible();
    await expect(plainRow.getByTestId("task-state-interrupted")).toHaveCount(0);

    const terminalRow = session.sidebarTaskItem("Terminal Fixture");
    await expect(terminalRow).toBeVisible();
    await expect(terminalRow.getByTestId("task-state-interrupted")).toHaveCount(0);

    // Reload: the marker must survive SSR hydration from the boot payload.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.sidebarTaskItem("Interrupted Fixture")).toBeVisible();
    await expect(
      session.sidebarTaskItem("Interrupted Fixture").getByTestId("task-state-interrupted"),
    ).toBeVisible();
    await expect(
      session.sidebarTaskItem("Plain Fixture").getByTestId("task-state-interrupted"),
    ).toHaveCount(0);
  });
});
