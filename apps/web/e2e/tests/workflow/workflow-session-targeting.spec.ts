import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import {
  createWorkflowAgentProfiles,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";

async function waitForAgentMarker(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
  sessionId: string,
  marker: string,
) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(marker),
        );
      },
      { timeout: 30_000, message: `session ${sessionId} did not receive ${marker}` },
    )
    .toBe(true);
}

test.describe("Workflow session targeting", () => {
  test("authors initial and source-session choices on desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Desktop Session Targets",
    );
    await apiClient.createWorkflowStep(workflow.id, "Plan", 0, { is_start_step: true });
    const implement = await apiClient.createWorkflowStep(workflow.id, "Implement", 1);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 2);
    await apiClient.updateWorkflowStep(implement.id, { agent_profile_id: profileA.id });
    await apiClient.updateWorkflowStep(review.id, { agent_profile_id: profileB.id });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Desktop Session Targets");
    await settings.selectStep(card, "Review");

    const selector = settings.stepAgentProfileSelect(card);
    await selector.click();
    await expect(testPage.getByTestId(`${review.id}-session-target-initial`)).toBeVisible();
    await expect(
      testPage.getByTestId(`${review.id}-session-target-step-${implement.id}`),
    ).toBeVisible();

    const lifecycle = testPage.locator(
      `[data-testid="${review.id}-profile-session-lifecycle-select"]:visible`,
    );
    await expect(lifecycle).toBeVisible();
    const search = testPage.getByPlaceholder("Search agent profiles...");
    await search.fill("does-not-exist");
    await expect(testPage.getByText("Profile not found.", { exact: true })).toBeVisible();
    await expect(lifecycle).toBeVisible();
    await search.fill("");

    await testPage.getByTestId(`${review.id}-session-target-initial`).click();
    await expect(selector).toContainText("Initial agent session");
    await selector.click();
    await lifecycle.click();
    await testPage.getByTestId(`${review.id}-profile-session-start-new`).click();
    await testPage.getByTestId(`${review.id}-profile-session-end-park`).click();
    await testPage.keyboard.press("Escape");
    await settings.saveChanges();

    const savedReview = (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
      (step) => step.id === review.id,
    );
    expect(savedReview).toMatchObject({
      session_target: { kind: "initial" },
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
    });
    expect(savedReview?.agent_profile_id ?? "").toBe("");

    await settings.goto(seedData.workspaceId);
    const reloadedCard = await settings.findWorkflowCard("Desktop Session Targets");
    await settings.selectStep(reloadedCard, "Review");
    await expect(settings.stepAgentProfileSelect(reloadedCard)).toContainText(
      "Initial agent session",
    );
  });

  test("delivers reuse and fresh initial targets to the intended conversations", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Initial Target Runtime");
    const plan = await apiClient.createWorkflowStep(workflow.id, "Plan", 0);
    const luna = await apiClient.createWorkflowStep(workflow.id, "Luna", 1);
    const reviewReuse = await apiClient.createWorkflowStep(workflow.id, "Review reuse", 2);
    const reviewFresh = await apiClient.createWorkflowStep(workflow.id, "Review fresh", 3);

    await apiClient.updateWorkflowStep(plan.id, {
      agent_profile_id: profileA.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(luna.id, {
      agent_profile_id: profileB.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(reviewReuse.id, {
      session_target: { kind: "initial" },
      prompt: 'e2e:message("initial-target-reuse")',
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(reviewFresh.id, {
      session_target: { kind: "initial" },
      profile_session_start_policy: "new",
      prompt: 'e2e:message("initial-target-fresh")',
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial target runtime",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: plan.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    const initialSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);
    await apiClient.moveTask(task.id, workflow.id, luna.id);
    const lunaSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileB.id);

    await apiClient.moveTask(task.id, workflow.id, reviewReuse.id);
    await waitForAgentMarker(apiClient, initialSessionId, "initial-target-reuse");
    const afterReuse = (await apiClient.listTaskSessions(task.id)).sessions;
    expect(afterReuse.filter((session) => session.agent_profile_id === profileA.id)).toHaveLength(
      1,
    );
    expect(afterReuse.find((session) => session.id === initialSessionId)?.is_primary).toBe(true);

    await apiClient.moveTask(task.id, workflow.id, reviewFresh.id);
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find(
            (session) =>
              session.agent_profile_id === profileA.id && session.id !== initialSessionId,
          )?.id;
        },
        { timeout: 30_000, message: "fresh initial-target session was not created" },
      )
      .toBeDefined();
    const finalSessions = (await apiClient.listTaskSessions(task.id)).sessions;
    const freshSession = finalSessions.find(
      (session) => session.agent_profile_id === profileA.id && session.id !== initialSessionId,
    );
    expect(freshSession).toBeDefined();
    await waitForAgentMarker(apiClient, freshSession!.id, "initial-target-fresh");
    expect(freshSession!.id).not.toBe(initialSessionId);
    expect(finalSessions.find((session) => session.id === lunaSessionId)?.state).toBe(
      "WAITING_FOR_INPUT",
    );
  });

  test("delivers a source-step target to its recorded conversation", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { agentId, profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
    const profileC = await apiClient.createAgentProfile(agentId, "Profile C (medium)", {
      model: "mock-fast",
    });
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Source Target Runtime");
    const plan = await apiClient.createWorkflowStep(workflow.id, "Plan", 0);
    const implement = await apiClient.createWorkflowStep(workflow.id, "Implement", 1);
    const verify = await apiClient.createWorkflowStep(workflow.id, "Verify", 2);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 3);

    await apiClient.updateWorkflowStep(plan.id, {
      agent_profile_id: profileA.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(implement.id, {
      agent_profile_id: profileB.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(verify.id, {
      agent_profile_id: profileC.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(review.id, {
      session_target: { kind: "step", step_id: implement.id },
      prompt: 'e2e:message("source-target-reuse")',
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Source target runtime",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: plan.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);
    await apiClient.moveTask(task.id, workflow.id, implement.id);
    const implementSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileB.id);
    await apiClient.moveTask(task.id, workflow.id, verify.id);
    await waitForWorkflowProfileSession(apiClient, task.id, profileC.id);

    await apiClient.moveTask(task.id, workflow.id, review.id);
    await waitForAgentMarker(apiClient, implementSessionId, "source-target-reuse");

    const finalSessions = (await apiClient.listTaskSessions(task.id)).sessions;
    expect(
      finalSessions.filter((session) => session.agent_profile_id === profileB.id),
    ).toHaveLength(1);
    expect(finalSessions.find((session) => session.id === implementSessionId)?.is_primary).toBe(
      true,
    );
    expect(finalSessions.find((session) => session.agent_profile_id === profileC.id)?.state).toBe(
      "WAITING_FOR_INPUT",
    );
  });

  test("surfaces a repair state when a source step is moved after its dependent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { profileA } = await createWorkflowAgentProfiles(apiClient);
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Repair Session Target");
    const source = await apiClient.createWorkflowStep(workflow.id, "Implement", 0);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(source.id, { agent_profile_id: profileA.id });
    await apiClient.updateWorkflowStep(review.id, {
      session_target: { kind: "step", step_id: source.id },
    });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Repair Session Target");
    await settings.selectStep(card, "Review");

    const sourceNode = settings.stepNodeByName(card, "Implement");
    const reviewNode = settings.stepNodeByName(card, "Review");
    const sourceHandle = sourceNode.locator("button").first();
    const sourceBox = await sourceHandle.boundingBox();
    const reviewBox = await reviewNode.boundingBox();
    expect(sourceBox).not.toBeNull();
    expect(reviewBox).not.toBeNull();
    await testPage.mouse.move(
      sourceBox!.x + sourceBox!.width / 2,
      sourceBox!.y + sourceBox!.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      reviewBox!.x + reviewBox!.width / 2,
      reviewBox!.y + reviewBox!.height / 2,
      {
        steps: 8,
      },
    );
    await testPage.mouse.up();

    await expect(settings.stepNodeByName(card, "Review")).toHaveAttribute(
      "data-workflow-invalid-session-target",
      "true",
    );
    await expect(testPage.getByTestId("workflow-session-target-repair")).toBeVisible();
    await expect(
      settings.floatingSave.getByRole("button", { name: "Save changes" }),
    ).toBeDisabled();
    await testPage.getByTestId("workflow-session-target-clear").click();
    await settings.saveChanges();

    const savedReview = (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
      (step) => step.id === review.id,
    );
    expect(savedReview?.session_target ?? null).toBeNull();
  });
});
