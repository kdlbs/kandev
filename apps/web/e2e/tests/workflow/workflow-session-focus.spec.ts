import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  createWorkflowSessionFocusScenario,
  waitForNewWorkflowSession,
} from "./workflow-session-focus-helpers";

test.describe("Workflow session focus", () => {
  test("focuses a new committed recipient on the task detail page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "new");
    await testPage.goto(`/t/${scenario.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.sessionTabBySessionId(scenario.secondarySessionId).click();
    await expect(session.activeChat()).toHaveAttribute(
      "data-session-id",
      scenario.secondarySessionId,
    );

    const moveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${scenario.task.id}/move$`),
    );
    await session.moveToWorkflowStep(scenario.destination);
    const response = await moveResponse;
    expect(response.ok()).toBe(true);
    expect((await response.json()).workflow_entry_identity).toMatch(/^entry:/);

    const destinationSessionId = await waitForNewWorkflowSession(apiClient, scenario.task.id, [
      scenario.sourceSessionId,
      scenario.secondarySessionId,
    ]);
    await expect(session.sessionTabBySessionId(destinationSessionId)).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.activeChat()).toHaveAttribute("data-session-id", destinationSessionId);
    await expect(session.sessionTabBySessionId(destinationSessionId)).toBeVisible();
  });

  test("focuses a reused recipient in the desktop task preview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "initial");
    await apiClient.saveUserSettings({
      enable_preview_on_click: true,
      workflow_filter_id: scenario.workflow.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle(`Workflow session focus initial task`);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();

    const preview = testPage.getByTestId("task-preview-panel");
    await expect(preview).toBeVisible({ timeout: 15_000 });
    const secondaryTab = preview.getByTestId(`preview-session-tab-${scenario.secondarySessionId}`);
    await expect(secondaryTab).toBeVisible({ timeout: 15_000 });
    await secondaryTab.click();
    await expect(secondaryTab).toHaveAttribute("data-state", "active");

    const moveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${scenario.task.id}/move$`),
    );
    await preview.getByTestId("workflow-stepper-minimal").hover();
    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    await expect(disclosure).toBeVisible();
    await waitForFiniteAnimations(testPage.locator('[data-slot="popover-content"]:visible'));
    const destinationRow = disclosure.getByTestId(
      `workflow-step-disclosure-row-${scenario.destination.id}`,
    );
    await expect(destinationRow).toBeVisible();
    const moveButton = destinationRow.getByTestId(
      `workflow-step-disclosure-move-${scenario.destination.id}`,
    );
    await expect(moveButton).toBeVisible();
    await moveButton.click();
    const response = await moveResponse;
    const movePayload = await response.json();
    expect(response.ok()).toBe(true);
    expect(movePayload.workflow_entry_identity).toMatch(/^entry:/);

    await expect
      .poll(async () => (await apiClient.getTask(scenario.task.id)).workflow_step_id)
      .toBe(scenario.destination.id);
    await expect
      .poll(
        async () =>
          (await apiClient.getTask(scenario.task.id)).metadata?.workflow_session_route?.phase,
      )
      .toBe("committed");
    const sourceTab = preview.getByTestId(`preview-session-tab-${scenario.sourceSessionId}`);
    await expect(sourceTab).toHaveAttribute("data-state", "active");
    await expect(secondaryTab).toHaveAttribute("data-state", "inactive");
  });

  test("keeps a later explicit session choice during a delayed move response", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "new");
    await testPage.goto(`/t/${scenario.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat()).toHaveAttribute("data-session-id", scenario.sourceSessionId);

    let releaseResponse!: () => void;
    let responseFetched!: () => void;
    const responseReleased = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    const backendMoveCommitted = new Promise<void>((resolve) => {
      responseFetched = resolve;
    });
    await testPage.route(`**/api/v1/tasks/${scenario.task.id}/move`, async (route) => {
      const response = await route.fetch();
      responseFetched();
      await responseReleased;
      await route.fulfill({ response });
    });

    try {
      const moveResponse = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/tasks/${scenario.task.id}/move$`),
      );
      await session.moveToWorkflowStep(scenario.destination);
      await backendMoveCommitted;

      // The destination may be arriving over WS while the HTTP response is
      // still held. This click is the user's later, explicit choice.
      await session.sessionTabBySessionId(scenario.secondarySessionId).click();
      releaseResponse();
      const response = await moveResponse;
      expect(response.ok()).toBe(true);

      const destinationSessionId = await waitForNewWorkflowSession(apiClient, scenario.task.id, [
        scenario.sourceSessionId,
        scenario.secondarySessionId,
      ]);
      await expect(session.sessionTabBySessionId(destinationSessionId)).toBeVisible({
        timeout: 15_000,
      });
      await expect(session.activeChat()).toHaveAttribute(
        "data-session-id",
        scenario.secondarySessionId,
      );
    } finally {
      releaseResponse();
      await testPage.unroute(`**/api/v1/tasks/${scenario.task.id}/move`);
    }
  });

  test("focuses a profile-only recipient instead of the pinned source session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "profile");
    await testPage.goto(`/t/${scenario.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.sessionTabBySessionId(scenario.secondarySessionId).click();
    await expect(session.activeChat()).toHaveAttribute(
      "data-session-id",
      scenario.secondarySessionId,
    );

    const moveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${scenario.task.id}/move$`),
    );
    await session.moveToWorkflowStep(scenario.destination);
    const response = await moveResponse;
    const payload = await response.json();
    expect(response.ok()).toBe(true);
    expect(payload.workflow_entry_identity).toMatch(/^entry:/);

    const destinationSessionId = await waitForNewWorkflowSession(apiClient, scenario.task.id, [
      scenario.sourceSessionId,
      scenario.secondarySessionId,
    ]);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", destinationSessionId);
    await expect(
      apiClient.getTask(scenario.task.id).then((task) => task.metadata?.workflow_session_route),
    ).resolves.toMatchObject({
      phase: "committed",
      destination_step_id: scenario.destination.id,
      destination_session_id: destinationSessionId,
    });
  });
});
