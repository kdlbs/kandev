import { expect, test } from "../../fixtures/test-base";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  RECOVERY_CHECK,
  RECOVERY_CLOSE,
  RECOVERY_OPEN,
  RECOVERY_RETRY,
  RECOVERY_STATUS,
  expectNoOpeningPrompt,
  expectCreateReady,
  expectOneRecoveryTask,
  expectOpeningPrompt,
  fillRecoveryDraft,
  installIncompleteLookupProjection,
  installLookupMisses,
  installLostResponseRoute,
  openDesktopCreateDialog,
  seedRecoveryWorkflow,
  selectRecoveryWorkflow,
  type CapturedCreate,
} from "./task-create-recovery-helpers";

useRegularMode();

function uniqueTitle(testInfo: { workerIndex: number; repeatEachIndex: number }, label: string) {
  return `Recovery ${label} ${testInfo.workerIndex}-${testInfo.repeatEachIndex}-${Date.now()}`;
}

test.describe("native task creation recovery", () => {
  test("retains one task when the browser loses the first create response", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(120_000);
    const title = uniqueTitle(testInfo, "start");
    const prompt = "Recover this exact brief without a second task.";
    const { workflowId } = await seedRecoveryWorkflow(
      apiClient,
      seedData,
      `Recovery start ${title}`,
    );
    const captured: CapturedCreate[] = [];
    await installLostResponseRoute(testPage, captured);
    await installLookupMisses(testPage);

    const dialog = await openDesktopCreateDialog(testPage);
    await selectRecoveryWorkflow(dialog, workflowId);
    await fillRecoveryDraft(dialog, title, prompt);
    const start = dialog.getByTestId("submit-start-agent");
    await expectCreateReady(dialog);

    const firstPost = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/api/v1/tasks",
    );
    await start.click();
    await firstPost;
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toBeVisible();
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toContainText(
      "Task creation is unconfirmed.",
    );
    await expect(dialog.getByTestId(RECOVERY_RETRY)).toBeVisible();
    await expect(dialog.getByTestId(RECOVERY_CHECK)).toBeVisible();
    await expect(dialog.getByTestId(RECOVERY_CLOSE)).toBeVisible();
    await prCapture.screenshot("task-create-recovery-desktop", {
      caption: "Desktop recovery status and actions after an uncertain native submission.",
    });
    expect(captured).toHaveLength(1);

    const lookup = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" && response.url().includes("/tasks/by-external-id"),
    );
    const retryPost = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/tasks",
    );
    const retryResponse = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId(RECOVERY_RETRY).click();
    await lookup;
    const retried = await retryPost;
    await retryResponse;
    const retriedBody = retried.postDataJSON() as { external_id?: string };
    expect(retriedBody.external_id).toBe(captured[0]!.external_id);

    await expect(dialog).toBeHidden();
    expect(captured).toHaveLength(2);
    expect(captured[1]!.id).toBe(captured[0]!.id);
    expect(captured[1]!.external_id).toBe(captured[0]!.external_id);
    await expectOneRecoveryTask(apiClient, seedData.workspaceId, title, captured[0]!.id);
    await expectOpeningPrompt(apiClient, captured[0]!.id, prompt);
  });

  test("restores an unresolved receipt on reload without creating anything", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const title = uniqueTitle(testInfo, "reload");
    const prompt = "Keep this receipt through a same-tab reload.";
    const { workflowId } = await seedRecoveryWorkflow(
      apiClient,
      seedData,
      `Recovery reload ${title}`,
    );
    const captured: CapturedCreate[] = [];
    await installLostResponseRoute(testPage, captured);

    const dialog = await openDesktopCreateDialog(testPage);
    await selectRecoveryWorkflow(dialog, workflowId);
    await fillRecoveryDraft(dialog, title, prompt);
    await expectCreateReady(dialog);
    await dialog.getByTestId("submit-start-agent").click();
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toBeVisible();
    expect(captured).toHaveLength(1);

    let postCount = 1;
    testPage.on("request", (request) => {
      if (request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/tasks") {
        postCount += 1;
      }
    });

    await testPage.reload();
    const reopened = await openDesktopCreateDialog(testPage);
    await expect(reopened.getByTestId(RECOVERY_STATUS)).toBeVisible();
    await expect(reopened.getByTestId(RECOVERY_CHECK)).toBeVisible();
    await expect(reopened.getByTestId(RECOVERY_CLOSE)).toBeVisible();
    expect(postCount, "restoration must not submit a create request").toBe(1);

    const statusLookup = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" && response.url().includes("/tasks/by-external-id"),
    );
    await reopened.getByTestId(RECOVERY_CHECK).click();
    await statusLookup;
    await expect(reopened.getByTestId(RECOVERY_STATUS)).toContainText("This task was created.");
    await expect(reopened.getByTestId(RECOVERY_OPEN)).toBeVisible();
    expect(postCount, "status lookup must not submit a create request").toBe(1);

    const taskRead = waitForHttp(testPage, "GET", /^\/api\/v1\/tasks\/[^/]+$/);
    await reopened.getByTestId(RECOVERY_OPEN).click();
    await taskRead;
    await expect(reopened).toBeHidden();
    expect(postCount, "opening the recovered task must not create another").toBe(1);
    await expectOneRecoveryTask(apiClient, seedData.workspaceId, title, captured[0]!.id);
  });

  test("reports an incomplete task and settles it without another launch", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const title = uniqueTitle(testInfo, "incomplete");
    const prompt = "Show the task while its creation is still settling.";
    const { workflowId } = await seedRecoveryWorkflow(
      apiClient,
      seedData,
      `Recovery incomplete ${title}`,
    );
    const captured: CapturedCreate[] = [];
    await installLostResponseRoute(testPage, captured);

    const dialog = await openDesktopCreateDialog(testPage);
    await selectRecoveryWorkflow(dialog, workflowId);
    await fillRecoveryDraft(dialog, title, prompt);
    await expectCreateReady(dialog);
    await dialog.getByTestId("submit-start-agent").click();
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toBeVisible();

    await expectOneRecoveryTask(apiClient, seedData.workspaceId, title, captured[0]!.id);
    const sessionsBefore = (await apiClient.listTaskSessions(captured[0]!.id)).sessions.length;

    const projection = await installIncompleteLookupProjection(testPage);
    const incompleteLookup = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" && response.url().includes("/tasks/by-external-id"),
    );
    await dialog.getByTestId(RECOVERY_CHECK).click();
    await incompleteLookup;
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toContainText(
      "This task exists. Creation is still in progress.",
    );
    await expect(dialog.getByTestId(RECOVERY_OPEN)).toBeVisible();
    expect((await apiClient.listTaskSessions(captured[0]!.id)).sessions.length).toBe(
      sessionsBefore,
    );

    projection.restore();
    const settledLookup = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" && response.url().includes("/tasks/by-external-id"),
    );
    await dialog.getByTestId(RECOVERY_CHECK).click();
    await settledLookup;
    await expect(dialog).toBeHidden();
    expect(captured).toHaveLength(1);
    expect((await apiClient.listTaskSessions(captured[0]!.id)).sessions.length).toBe(
      sessionsBefore,
    );
  });

  test("retries create-only submissions without an opening prompt", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const title = uniqueTitle(testInfo, "create-only");
    const prompt = "Create this task without starting its agent.";
    const { workflowId } = await seedRecoveryWorkflow(
      apiClient,
      seedData,
      `Recovery create-only ${title}`,
    );
    const captured: CapturedCreate[] = [];
    await installLostResponseRoute(testPage, captured);
    await installLookupMisses(testPage);

    const dialog = await openDesktopCreateDialog(testPage);
    await selectRecoveryWorkflow(dialog, workflowId);
    await fillRecoveryDraft(dialog, title, prompt);
    await expectCreateReady(dialog);
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog.getByTestId(RECOVERY_STATUS)).toBeVisible();
    expect(captured).toHaveLength(1);

    const retryResponse = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId(RECOVERY_RETRY).click();
    await retryResponse;
    await expect(dialog).toBeHidden();
    expect(captured).toHaveLength(2);
    expect(captured[1]!.id).toBe(captured[0]!.id);
    expect(captured[1]!.external_id).toBe(captured[0]!.external_id);
    await expect
      .poll(
        async () =>
          (await apiClient.listTasks(seedData.workspaceId)).tasks.filter((t) => t.title === title)
            .length,
        { timeout: 30_000 },
      )
      .toBe(1);
    await expectNoOpeningPrompt(apiClient, captured[0]!.id);
  });
});

test("creation-pending inspection does not ensure or start a session", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
  const title = uniqueTitle(testInfo, "pending-route");
  const { workflowId, stepId } = await seedRecoveryWorkflow(
    apiClient,
    seedData,
    `Recovery pending ${title}`,
  );
  const seeded = await apiClient.seedTask(seedData.workspaceId, title, {
    workflow_id: workflowId,
    workflow_step_id: stepId,
    state: "CREATED",
  });

  await testPage.goto(`/t/${seeded.task_id}?creation=pending`);
  await expect(testPage.getByText(title).first()).toBeVisible();
  await dwell(
    testPage,
    3000,
    "negative-assertion",
    "a creation-pending task must not auto-ensure its session",
  );
  expect((await apiClient.listTaskSessions(seeded.task_id)).sessions).toHaveLength(0);
  await expect(testPage.getByText("Couldn't start a session")).toHaveCount(0);

  // Control: the same task outside the gate attempts ensure, which this
  // under-specified seeded task cannot complete. The visible failure proves
  // the ordinary task route would have raced the original creation owner.
  await testPage.goto(`/t/${seeded.task_id}`);
  await expect(testPage.getByText("Couldn't start a session")).toBeVisible({ timeout: 30_000 });
});
