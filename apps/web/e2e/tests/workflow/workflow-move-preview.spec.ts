import { expectPreviewFooter } from "./workflow-move-preview-assertions";
import type { Page, Request } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  createWorkflowAgentProfiles,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";
import { waitForWorkflowMoveLifecycle } from "../task/task-workflow-agent-overrides-helpers";
import { seedMoveOverrideFixture } from "./workflow-step-move-overrides-helpers";
import {
  applyHarmlessPreviewUpdate,
  movePreviewRequestPredicate,
} from "./workflow-move-preview-stability-helpers";

function waitForPreviewResponse(page: Page, taskId: string) {
  return page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === `/api/v1/tasks/${taskId}/move-preview` &&
      response.ok(),
  );
}

async function openStepPreview(page: Page, taskId: string, stepName: string) {
  const response = waitForPreviewResponse(page, taskId);
  await page.getByTestId(`workflow-step-${stepName}`).hover();
  const popover = page.locator('[data-testid="workflow-step-popover"]:visible');
  await expect(popover).toBeVisible();
  await expect(popover.getByTestId("workflow-move-preview")).toBeVisible({ timeout: 15_000 });
  return { popover, preview: await response.then((value) => value.json()) };
}

test.describe("Workflow move preview", () => {
  test("shows a retained session model override before an in-place move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedMoveOverrideFixture(
      testPage,
      apiClient,
      seedData,
      "Desktop Move Preview Override",
    );
    await apiClient.setSessionModel(fixture.primarySessionId, "mock-slow");
    await testPage.reload();
    await fixture.session.waitForLoad();

    const { popover, preview } = await openStepPreview(testPage, fixture.taskId, "Verify");
    await expectPreviewFooter(popover, "workflow-step-move-here");
    expect(preview.outcome).toBe("reuse_current");
    expect(preview.recipient.session_id).toBe(fixture.primarySessionId);
    expect(preview.model.after.label).toBe("mock-slow");
    expect(preview.model.after_source).toBe("override");
    await expect(popover.getByTestId("workflow-move-preview")).toContainText("override retained");

    const moveRequest = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === `/api/v1/tasks/${fixture.taskId}/move`,
    );
    await popover.getByTestId("workflow-step-move-here").click();
    expect((await moveRequest).postDataJSON()).toMatchObject({
      workflow_step_id: fixture.targetStepId,
    });
    await expect
      .poll(() => apiClient.getTask(fixture.taskId).then((task) => task.primary_session_id))
      .toBe(fixture.primarySessionId);
  });

  test("keeps harmless updates stable and refreshes for a model change", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedMoveOverrideFixture(
      testPage,
      apiClient,
      seedData,
      "Desktop Move Preview Stability",
    );
    const isMovePreviewRequest = movePreviewRequestPredicate(fixture.taskId, fixture.targetStepId);
    let requestCount = 0;
    const requestListener = (request: Request) => {
      if (isMovePreviewRequest(request)) requestCount += 1;
    };
    testPage.on("request", requestListener);

    try {
      const { popover } = await openStepPreview(testPage, fixture.taskId, "Verify");
      expect(requestCount).toBe(1);

      let unexpectedRequest = false;
      void testPage
        .waitForRequest(isMovePreviewRequest)
        .then(() => {
          unexpectedRequest = true;
        })
        .catch(() => undefined);
      for (const revision of [1, 2, 3]) {
        await applyHarmlessPreviewUpdate(testPage, fixture.taskId, revision);
      }
      await dwell(
        testPage,
        500,
        "negative-assertion",
        "observe no move-preview refresh after harmless task updates",
      );
      expect(unexpectedRequest).toBe(false);
      expect(requestCount).toBe(1);
      await expect(popover.getByTestId("workflow-move-preview")).toBeVisible();
      await expect(popover.getByTestId("workflow-move-preview-loading")).toHaveCount(0);

      const movePreviewUrl = `**/api/v1/tasks/${fixture.taskId}/move-preview`;
      let releaseHeldRequest: (() => void) | undefined;
      let resolveHeldRequest!: () => void;
      const heldRequest = new Promise<void>((resolve) => {
        resolveHeldRequest = resolve;
      });
      let held = false;
      await testPage.route(movePreviewUrl, async (route) => {
        if (route.request().method() === "POST" && !held) {
          held = true;
          resolveHeldRequest();
          await new Promise<void>((resolve) => {
            releaseHeldRequest = resolve;
          });
        }
        await route.continue();
      });

      try {
        const modelUpdate = apiClient.setSessionModel(fixture.primarySessionId, "mock-slow");
        await heldRequest;
        expect(requestCount).toBe(2);
        await expect(popover.getByTestId("workflow-move-preview-loading")).toBeVisible();
        releaseHeldRequest?.();
        await modelUpdate;
        await expect(popover.getByTestId("workflow-move-preview")).toContainText("mock-slow");
      } finally {
        releaseHeldRequest?.();
        await testPage.unroute(movePreviewUrl);
      }
    } finally {
      testPage.off("request", requestListener);
    }
  });

  test("matches other-session reuse and fresh-session creation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { agentId, profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
    const profileC = await apiClient.createAgentProfile(agentId, "Profile C (new)", {
      model: "mock-fast",
    });
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Move Preview Routing");
    const source = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
      is_start_step: true,
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    const bridge = await apiClient.createWorkflowStep(workflow.id, "Bridge", 1, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.createWorkflowStep(workflow.id, "Return A", 2, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.createWorkflowStep(workflow.id, "Fresh C", 3, {
      agent_profile_id: profileC.id,
      profile_session_start_policy: "new",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Move Preview Routing Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: source.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    const initialSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);
    await apiClient.moveTask(task.id, workflow.id, bridge.id);
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    await waitForWorkflowProfileSession(apiClient, task.id, profileB.id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const returnPreview = await openStepPreview(testPage, task.id, "Return A");
    expect(returnPreview.preview.outcome).toBe("reuse_other");
    expect(returnPreview.preview.recipient.session_id).toBe(initialSessionId);
    expect(returnPreview.preview.model.after.known).toBe(true);
    const returnMove = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === `/api/v1/tasks/${task.id}/move`,
    );
    await returnPreview.popover.getByTestId("workflow-step-move-here").click();
    await returnMove;
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    await expect
      .poll(() => apiClient.getTask(task.id).then((current) => current.primary_session_id))
      .toBe(initialSessionId);

    const freshPreview = await openStepPreview(testPage, task.id, "Fresh C");
    expect(freshPreview.preview.outcome).toBe("create_new");
    expect(freshPreview.preview.recipient.session_id ?? "").toBe("");
    expect(freshPreview.preview.model.after.known).toBe(true);
    const freshMove = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === `/api/v1/tasks/${task.id}/move`,
    );
    await freshPreview.popover.getByTestId("workflow-step-move-here").click();
    await freshMove;
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    const freshSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileC.id);
    expect(freshSessionId).not.toBe(initialSessionId);
  });
});
