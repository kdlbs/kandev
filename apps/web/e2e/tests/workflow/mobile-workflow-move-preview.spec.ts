import type { Request } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { dwell } from "../../helpers/causal-waits";
import { waitForLatestSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import {
  applyHarmlessPreviewUpdate,
  movePreviewRequestPredicate,
} from "./workflow-move-preview-stability-helpers";

type PreviewStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessionsByTask: {
        loadedByTaskId: Record<string, boolean>;
        itemsByTaskId: Record<string, Array<{ id: string; state?: string }>>;
      };
      sessionModels: {
        bySessionId: Record<string, { currentModelId: string; models: Array<{ modelId: string }> }>;
      };
    };
  };
};

test.describe("mobile: workflow move preview", () => {
  test("shows the same prediction and details inside the touch drawer", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Move Preview Task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForLatestSessionDone(
      apiClient,
      task.id,
      1,
      "Mobile move preview task must finish before navigation",
      30_000,
    );
    const sortedSteps = [...seedData.steps].sort((left, right) => left.position - right.position);
    const currentIndex = sortedSteps.findIndex((step) => step.id === seedData.startStepId);
    const targetStep = sortedSteps[currentIndex + 1] ?? sortedSteps[currentIndex - 1];
    if (!targetStep) throw new Error("mobile move preview requires an adjacent target");

    const kanban = new KanbanPage(tabletTestPage);
    await kanban.goto();
    const { settings } = await apiClient.getUserSettings();
    const previousPreviewOnClick = settings.enable_preview_on_click === true;
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    try {
      await kanban.goto();
      const card = kanban.taskCardByTitle("Mobile Move Preview Task");
      await expect(card).toBeVisible({ timeout: 10_000 });
      await card.tap();

      const previewPanel = tabletTestPage.getByTestId("task-preview-panel");
      await expect(previewPanel).toBeVisible({ timeout: 10_000 });
      const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
      await expect(trigger).toBeVisible({ timeout: 15_000 });

      // The preview revision includes the task-session projection. Wait for
      // that projection before opening the drawer, so its initial load cannot
      // invalidate the first request and look like a refresh caused by the
      // harmless bookkeeping updates below.
      await tabletTestPage.waitForFunction(
        ({ taskId, sessionId }) => {
          const store = (window as PreviewStoreWindow).__KANDEV_E2E_STORE__;
          if (!store) return false;
          const state = store.getState();
          const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
          const primarySession = sessions.find((session) => session.id === sessionId);
          const modelCatalog = state.sessionModels.bySessionId[sessionId];
          return (
            state.taskSessionsByTask.loadedByTaskId[taskId] === true &&
            (primarySession?.state === "WAITING_FOR_INPUT" ||
              primarySession?.state === "COMPLETED") &&
            modelCatalog !== undefined &&
            modelCatalog.currentModelId !== "" &&
            modelCatalog.models.length > 0
          );
        },
        { taskId: task.id, sessionId: task.primary_session_id ?? task.session_id ?? "" },
        { timeout: 15_000 },
      );

      const isMovePreviewRequest = movePreviewRequestPredicate(task.id, targetStep.id);
      let requestCount = 0;
      const requestListener = (request: Request) => {
        if (isMovePreviewRequest(request)) {
          requestCount += 1;
        }
      };
      tabletTestPage.on("request", requestListener);

      try {
        await trigger.tap();

        const drawer = tabletTestPage.locator('[data-slot="drawer-content"][data-state="open"]');
        await expect(drawer).toBeVisible();
        const row = drawer.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`);
        await expect(row).toBeVisible();
        const preview = row.getByTestId("workflow-move-preview");
        await expect(preview).toBeVisible({ timeout: 15_000 });
        await expect(preview).toContainText("Reuse current session");
        await expect(preview).toContainText("mock-fast");
        expect(requestCount).toBe(1);

        const move = row.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`);
        await expect(move).toBeVisible();
        await expect(row.getByTestId("workflow-move-preview-details")).toHaveCount(0);
        const labelBox = await row.getByText(targetStep.name, { exact: true }).boundingBox();
        const moveBox = await move.boundingBox();
        expect(labelBox).not.toBeNull();
        expect(moveBox).not.toBeNull();
        // Text glyph bounds and the 44px touch target use different font
        // metrics on Chromium. Keep the check strict enough to catch a row
        // that is visibly off-center while allowing the normal glyph offset.
        expect(
          Math.abs(labelBox!.y + labelBox!.height / 2 - moveBox!.y - moveBox!.height / 2),
        ).toBeLessThan(8);
        await expect(preview).toHaveCSS("text-align", "left");
        const detailsToggle = row.getByTestId(`workflow-step-disclosure-options-${targetStep.id}`);
        const toggleBox = await detailsToggle.boundingBox();
        expect(toggleBox).not.toBeNull();
        if (!toggleBox) return;
        expect(toggleBox.height).toBeGreaterThanOrEqual(44);
        expect(toggleBox.width).toBeGreaterThanOrEqual(44);
        await detailsToggle.tap();
        await expect(row.getByTestId("workflow-move-preview-details")).toBeVisible();

        let unexpectedRequest = false;
        void tabletTestPage
          .waitForRequest(isMovePreviewRequest)
          .then(() => {
            unexpectedRequest = true;
          })
          .catch(() => undefined);
        for (const revision of [1, 2, 3]) {
          await applyHarmlessPreviewUpdate(tabletTestPage, task.id, revision);
        }
        await dwell(
          tabletTestPage,
          500,
          "negative-assertion",
          "observe no move-preview refresh after harmless touch updates",
        );
        expect(unexpectedRequest).toBe(false);
        expect(requestCount).toBe(1);
        await expect(row.getByTestId("workflow-move-preview-details")).toBeVisible();
        await expect(row.getByTestId("workflow-move-preview-loading")).toHaveCount(0);

        const movePreviewUrl = `**/api/v1/tasks/${task.id}/move-preview`;
        let releaseHeldRequest: (() => void) | undefined;
        let resolveHeldRequest!: () => void;
        const heldRequest = new Promise<void>((resolve) => {
          resolveHeldRequest = resolve;
        });
        let held = false;
        await tabletTestPage.route(movePreviewUrl, async (route) => {
          if (isMovePreviewRequest(route.request()) && !held) {
            held = true;
            resolveHeldRequest();
            await new Promise<void>((resolve) => {
              releaseHeldRequest = resolve;
            });
          }
          await route.continue();
        });
        try {
          const currentTask = await apiClient.getTask(task.id);
          const modelUpdate = apiClient.setSessionModel(
            currentTask.primary_session_id,
            "mock-slow",
          );
          await heldRequest;
          expect(requestCount).toBe(2);
          await expect(row.getByTestId("workflow-move-preview-loading")).toBeVisible();
          releaseHeldRequest?.();
          await modelUpdate;
          await expect(preview).toContainText("mock-slow");
        } finally {
          releaseHeldRequest?.();
          await tabletTestPage.unroute(movePreviewUrl);
        }

        for (const testId of [
          "workflow-step-disclosure-options-" + targetStep.id,
          "workflow-step-disclosure-move-" + targetStep.id,
        ]) {
          const box = await drawer.getByTestId(testId).boundingBox();
          expect(box).not.toBeNull();
          if (!box) return;
          expect(box.height).toBeGreaterThanOrEqual(44);
        }
        await assertNoDocumentHorizontalOverflow(tabletTestPage);

        await tabletTestPage.keyboard.press("Escape");
        await expect(
          tabletTestPage.locator('[data-slot="drawer-content"][data-state="open"]'),
        ).toHaveCount(0);
        await expect(trigger).toBeFocused();
      } finally {
        tabletTestPage.off("request", requestListener);
      }
    } finally {
      await apiClient.saveUserSettings({ enable_preview_on_click: previousPreviewOnClick });
    }
  });
});
