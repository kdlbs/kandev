import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Workflow auto-advance", () => {
  /**
   * Seeds a 3-step workflow (Inbox -> In Progress -> Done).
   * Configures In Progress with on_enter: auto_start_agent and a custom step
   * prompt routed to the mock agent, plus on_turn_complete: move_to_step(Done).
   */
  test("auto_start_agent with step custom prompt; on_turn_complete advances to next column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Automation Test Workflow",
    );

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const inProgressStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(inProgressStep.id, {
      prompt: 'e2e:delay(3000)\ne2e:message("delayed step response")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Auto Agent Workflow Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(task.id, workflow.id, inProgressStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardInProgress = kanban.taskCardInColumn("Auto Agent Workflow Task", inProgressStep.id);
    await expect(cardInProgress).toBeVisible({ timeout: 15_000 });

    const cardInDone = kanban.taskCardInColumn("Auto Agent Workflow Task", doneStep.id);
    await expect(cardInDone).toBeVisible({ timeout: 30_000 });

    await cardInDone.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });
    await expect(
      session.activeChat().getByText("delayed step response", { exact: true }),
    ).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 15_000 });
    await expect(session.sidebarSection("Turn Finished")).toBeVisible();
  });

  test("does not retrigger previous step prompt after auto-advancing into Done", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Final Step No Retrigger Workflow",
    );

    const startStep = await apiClient.createWorkflowStep(workflow.id, "Start", 0);
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Work", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(startStep.id, {
      events: {
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    const workMarker = "final-step-work-response-once";
    await apiClient.updateWorkflowStep(workStep.id, {
      prompt: `e2e:message("${workMarker}")\n{{task_prompt}}`,
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Final Done Retrigger Task", {
      workflow_id: workflow.id,
      workflow_step_id: startStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await session.waitForLoad();
    await expect(session.stepperStep("Start")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    await session.sendMessageViaButton("/e2e:simple-message");

    await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
      timeout: 45_000,
    });
    await expect(session.activeChat().getByText(workMarker, { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 30_000 });

    const taskSessions = await apiClient.listTaskSessions(task.id);
    const primarySession = taskSessions.sessions.find((candidate) => candidate.is_primary);
    expect(primarySession, "expected a primary task session").toBeTruthy();

    const { messages } = await apiClient.listSessionMessages(primarySession!.id);
    const workResponses = messages.filter(
      (message) => message.author_type === "agent" && message.content.includes(workMarker),
    );
    const workPrompts = messages.filter(
      (message) => message.author_type === "user" && message.content.includes(workMarker),
    );

    expect(workResponses).toHaveLength(1);
    expect(workPrompts).toHaveLength(1);
  });
});
