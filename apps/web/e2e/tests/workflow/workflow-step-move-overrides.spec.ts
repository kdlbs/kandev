import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  fillMoveOverrides,
  MOVE_INSTRUCTIONS,
  seedMoveOverrideFixture,
  TARGET_STEP_MESSAGE,
  waitForMoveRequest,
} from "./workflow-step-move-overrides-helpers";

type SpecApiClient = Parameters<typeof seedMoveOverrideFixture>[1];

/** Count messages in a session whose content includes the given needle. */
async function countSessionMessages(
  apiClient: SpecApiClient,
  sessionId: string,
  needle: string,
): Promise<number> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter((message) => message.content.includes(needle)).length;
}

async function sessionState(
  apiClient: SpecApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId)?.state ?? "";
}

/** Poll until the task has committed to the given workflow step. */
async function waitForStepCommitted(
  apiClient: SpecApiClient,
  taskId: string,
  stepId: string,
): Promise<void> {
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).workflow_step_id, {
      timeout: 20_000,
      message: `waiting for task ${taskId} to commit to step ${stepId}`,
    })
    .toBe(stepId);
}

/**
 * Open the compact anchored move-options surface from the desktop stepper.
 * Fine-pointer hover reveals it; the one-shot fields are opt-in.
 */
async function openStepperMoveOptions(testPage: Parameters<typeof waitForMoveRequest>[0]) {
  await testPage.getByTestId("workflow-step-Verify").hover();
  await expect(testPage.getByTestId("workflow-step-popover")).toBeVisible();
  await testPage.getByTestId("workflow-step-move-options-trigger").click();
  await expect(testPage.getByTestId("workflow-move-instructions")).toBeVisible();
}

test("moves in place with one-shot options from the desktop stepper", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Move Override",
  );
  const defaultsBefore = (await apiClient.listWorkflowSteps(fixture.workflowId)).steps.find(
    (step) => step.id === fixture.targetStepId,
  );
  expect(defaultsBefore).toBeDefined();

  await openStepperMoveOptions(testPage);
  await fillMoveOverrides(testPage);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-step-move-here").click();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(
    fixture.session.chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first(),
  ).toBeVisible({ timeout: 30_000 });

  // The move applies in place: the task keeps its single primary session.
  expect((await apiClient.getTask(fixture.taskId)).primary_session_id).toBe(
    fixture.primarySessionId,
  );
  await expect
    .poll(() => countSessionMessages(apiClient, fixture.primarySessionId, MOVE_INSTRUCTIONS), {
      timeout: 30_000,
      message: "instructions were not delivered exactly once to the primary session",
    })
    .toBe(1);

  // The overlay is one-shot: the target step's durable configuration is untouched.
  const defaultsAfter = (await apiClient.listWorkflowSteps(fixture.workflowId)).steps.find(
    (step) => step.id === fixture.targetStepId,
  );
  expect(defaultsAfter).toEqual(defaultsBefore);
});

test("uses the desktop next-step anchored form for the same one-shot move contract", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Sidecar Move Override",
  );
  const nextStepButton = testPage.getByTestId("proceed-next-step");
  await expect(nextStepButton).toBeVisible();
  await nextStepButton.hover();
  await expect(testPage.getByTestId("proceed-next-step-options")).toBeVisible();
  await fillMoveOverrides(testPage);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").click();

  expect((await moveRequest).postDataJSON()).toMatchObject({
    workflow_step_id: fixture.targetStepId,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  expect((await apiClient.getTask(fixture.taskId)).primary_session_id).toBe(
    fixture.primarySessionId,
  );
  await expect
    .poll(() => countSessionMessages(apiClient, fixture.primarySessionId, MOVE_INSTRUCTIONS), {
      timeout: 30_000,
      message: "instructions were not delivered exactly once to the primary session",
    })
    .toBe(1);
});

test("skip step prompt with instructions delivers only the instructions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Skip With Instructions",
  );

  await openStepperMoveOptions(testPage);
  await fillMoveOverrides(testPage, { resetContext: false, skipStepPrompt: true });
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-step-move-here").click();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      skip_step_prompt: true,
      instructions: MOVE_INSTRUCTIONS,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(
    fixture.session.chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first(),
  ).toBeVisible({ timeout: 30_000 });
  await expect
    .poll(() => countSessionMessages(apiClient, fixture.primarySessionId, MOVE_INSTRUCTIONS), {
      timeout: 30_000,
      message: "instructions were not delivered exactly once to the primary session",
    })
    .toBe(1);

  // The step's durable prompt (and its e2e:message) is suppressed for this entry.
  await dwell(testPage, 2_000, "negative-assertion", "target step prompt must stay suppressed");
  expect(await countSessionMessages(apiClient, fixture.primarySessionId, TARGET_STEP_MESSAGE)).toBe(
    0,
  );
});

test("skip step prompt without instructions starts no turn and parks for input", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Skip No Instructions",
  );

  await openStepperMoveOptions(testPage);
  await fillMoveOverrides(testPage, {
    resetContext: false,
    skipStepPrompt: true,
    instructions: "",
  });
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-step-move-here").click();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      skip_step_prompt: true,
    },
  });
  await waitForStepCommitted(apiClient, fixture.taskId, fixture.targetStepId);

  // No prompt and no instructions: no turn starts, the session parks for input,
  // and the target step's durable message never appears.
  await expect
    .poll(() => sessionState(apiClient, fixture.taskId, fixture.primarySessionId), {
      timeout: 30_000,
      message: "skip-with-no-instructions session did not settle",
    })
    .toBe("WAITING_FOR_INPUT");
  await dwell(testPage, 2_000, "negative-assertion", "no turn should start for a bare skip move");
  expect(await countSessionMessages(apiClient, fixture.primarySessionId, TARGET_STEP_MESSAGE)).toBe(
    0,
  );
  expect(await sessionState(apiClient, fixture.taskId, fixture.primarySessionId)).toBe(
    "WAITING_FOR_INPUT",
  );
});
