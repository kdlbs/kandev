import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import {
  canvasHref,
  type CanvasRecord,
  enableCanvasFeature,
  expectCanvasFrameFillsHost,
  getCanvas,
  publishTaskCanvas,
  removeCanvas,
  promoteCanvas,
  seedTaskCanvas,
  waitForTaskCanvas,
} from "./canvas-fixture";

async function approvePendingCanvasThroughHost(
  page: Page,
  apiClient: ApiClient,
  canvas: CanvasRecord,
): Promise<CanvasRecord> {
  const pendingReleaseId = canvas.pending_release?.id;
  if (!pendingReleaseId) throw new Error("The canvas has no pending release to approve.");

  await expect(page).toHaveURL(new RegExp(`${canvasHref(canvas.id)}$`), { timeout: 30_000 });
  await expect(page.getByTestId("canvas-host-state")).toHaveText("Permission review required");
  await page.getByTestId("canvas-mobile-actions").tap();
  const actionsSheet = page.getByTestId("canvas-mobile-actions-sheet");
  await expect(actionsSheet).toBeVisible();
  await actionsSheet.getByRole("button", { name: "Releases and permissions", exact: true }).tap();
  const releasesDialog = page.getByTestId("canvas-releases-dialog");
  await expect(releasesDialog).toBeVisible();
  await expect(
    releasesDialog.getByTestId(`canvas-release-permissions-${pendingReleaseId}`),
  ).toBeVisible();
  await releasesDialog.getByRole("button", { name: "Approve release", exact: true }).tap();
  await expect(page.getByTestId("canvas-host-state")).toHaveText("Ready", { timeout: 20_000 });

  let approvedCanvas: CanvasRecord | null = null;
  await expect
    .poll(
      async () => {
        approvedCanvas = await getCanvas(apiClient, canvas.id);
        return approvedCanvas?.active_release_status === "valid";
      },
      { timeout: 30_000, message: "The mobile approval did not activate the canvas release." },
    )
    .toBe(true);
  if (!approvedCanvas) throw new Error("The approved canvas record was empty.");
  return approvedCanvas;
}

test.describe("Plugin-backed canvases on mobile", () => {
  test("creates a scratch canvas task from workspace settings with editable choices", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    const canvasIds: string[] = [];
    let taskId: string | undefined;
    let alternateWorkflowId: string | undefined;
    try {
      const { executors } = await apiClient.listExecutors();
      const localExecutor = executors.find((executor) =>
        ["local", "local_pc"].includes(executor.type),
      );
      const localProfile = localExecutor?.profiles?.[0];
      expect(
        localProfile,
        "a direct local executor profile is required by the fixture",
      ).toBeDefined();

      const alternateWorkflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "E2E Canvas Alternative Workflow",
        "simple",
      );
      alternateWorkflowId = alternateWorkflow.id;

      await testPage.goto(
        `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/canvases`,
      );
      await expect(testPage.getByTestId("workspace-canvases-page")).toBeVisible();
      await testPage.getByTestId("settings-create-canvas").tap();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("source-mode-scratch")).toHaveAttribute(
        "aria-checked",
        "true",
      );
      await expect(dialog.getByTestId("source-mode-workspace")).toBeVisible();
      await expect(dialog.getByTestId("executor-profile-selector")).toContainText(
        localProfile!.name,
      );

      const agentSelector = dialog.getByTestId("agent-profile-selector");
      await expect(agentSelector).toBeEnabled();
      await agentSelector.tap();
      await expect(testPage.getByRole("option").first()).toBeVisible();
      await testPage.keyboard.press("Escape");

      const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
      await expect(workflowSelector).toBeVisible();
      await workflowSelector.tap();
      await expect(
        testPage.getByRole("button", { name: alternateWorkflow.name, exact: true }),
      ).toBeVisible();
      await expect(
        testPage.getByRole("button", { name: "E2E Workflow", exact: true }).last(),
      ).toBeVisible();
      // The open selector keeps its current value as the trigger and renders
      // the same label again as a popover option. The last matching button is
      // the option, so this remains deterministic under strict locators.
      await testPage.getByRole("button", { name: "E2E Workflow", exact: true }).last().tap();

      const canvasTitle = "E2E Guided Canvas";
      const taskTitle = "E2E Guided Canvas Task";
      const description = [
        `e2e:mcp:kandev:create_canvas_kandev(${JSON.stringify({
          title: canvasTitle,
          summary: "Canvas created through the guided settings task flow.",
        })})`,
        'e2e:message("Canvas created from settings.")',
      ].join("\n");
      await dialog.getByTestId("task-title-input").fill(taskTitle);
      await dialog.getByTestId("task-description-input").fill(description);

      const responsePromise = testPage.waitForResponse(
        (response) =>
          response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
      );
      const startAgent = dialog.getByTestId("submit-start-agent");
      await expect(startAgent).toBeEnabled({ timeout: 30_000 });
      await startAgent.tap();
      const response = await responsePromise;
      const responseBody = await response.text();
      expect(response.status(), responseBody).toBe(200);
      const created = JSON.parse(responseBody) as { id: string; session_id?: string };
      taskId = created.id;
      expect(taskId).toBeTruthy();

      await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}(?:[?]|$)`));
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 45_000 });

      let taskSessionId = created.session_id ?? "";
      await expect
        .poll(
          async () => {
            if (!taskSessionId) {
              const { sessions } = await apiClient.listTaskSessions(taskId!);
              taskSessionId = sessions[0]?.id ?? "";
            }
            return taskSessionId;
          },
          { timeout: 30_000, message: "The guided canvas task did not expose a session." },
        )
        .not.toBe("");

      const canvas = await waitForTaskCanvas(apiClient, taskId, canvasTitle);
      canvasIds.push(canvas.id);
      const published = await publishTaskCanvas({
        apiClient,
        taskId,
        taskSessionId,
        session,
        canvas,
        useMobileSubmit: true,
      });

      await approvePendingCanvasThroughHost(testPage, apiClient, published);

      const createdTask = await apiClient.getTask(taskId);
      expect(createdTask.repositories ?? []).toHaveLength(0);
      const { sessions } = await apiClient.listTaskSessions(taskId);
      expect(
        sessions.find((candidate) => candidate.id === taskSessionId)?.executor_profile_id,
      ).toBe(localProfile!.id);
    } finally {
      await Promise.all(canvasIds.map((canvasId) => removeCanvas(apiClient, canvasId)));
      if (alternateWorkflowId)
        await apiClient.deleteWorkflow(alternateWorkflowId).catch(() => undefined);
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await releaseFeature();
    }
  });

  test("automatically opens a pending release and approves it through mobile host controls", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    const canvasIds: string[] = [];
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasIds.push(seeded.canvas.id);

      await expect(testPage).toHaveURL(new RegExp(`${canvasHref(seeded.canvas.id)}$`), {
        timeout: 30_000,
      });
      await approvePendingCanvasThroughHost(testPage, apiClient, seeded.canvas);
    } finally {
      await Promise.all(canvasIds.map((canvasId) => removeCanvas(apiClient, canvasId)));
      await releaseFeature();
    }
  });

  test("uses a focused route, workspace navigation, and an inset action drawer", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    const canvasIds: string[] = [];
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasIds.push(seeded.canvas.id);
      const activeCanvas = await approvePendingCanvasThroughHost(
        testPage,
        apiClient,
        seeded.canvas,
      );
      const activeReleaseId = activeCanvas.active_release_id;

      await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);

      await testPage.goto(canvasHref(activeCanvas.id));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 20_000 });
      await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);
      await expect(testPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 20_000 },
      );
      await expectCanvasFrameFillsHost(testPage);
      const fixture = testPage.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(fixture.getByTestId("canvas-fixture-script")).toHaveText("inline-ready");
      await expect(fixture.getByTestId("canvas-fixture-appearance-mode")).toHaveText("light");
      await expect(fixture.getByTestId("canvas-fixture-appearance-color-scheme")).toHaveText(
        "light",
      );
      await expect(fixture.getByTestId("canvas-fixture-appearance-background")).not.toHaveText(
        "loading",
      );
      await expect(fixture.getByTestId("canvas-fixture-context")).toHaveText(seeded.taskId);
      await expect(fixture.getByTestId("canvas-fixture-sse-status")).toHaveText("connected");

      const actionsButton = testPage.getByTestId("canvas-mobile-actions");
      await expect(actionsButton).toBeVisible();
      expect((await actionsButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await actionsButton.tap();

      const actionsSheet = testPage.getByTestId("canvas-mobile-actions-sheet");
      await expect(actionsSheet).toBeVisible();
      const promoteButton = actionsSheet.getByRole("button", {
        name: "Promote canvas",
        exact: true,
      });
      await expect(promoteButton).toBeVisible();
      expect((await promoteButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await promoteButton.tap();

      const promotionDialog = testPage.getByTestId("canvas-promotion-dialog");
      await expect(promotionDialog).toBeVisible();
      await expect(promotionDialog.getByTestId("canvas-promotion-target-scope")).toHaveText(
        "workspace",
      );
      await promotionDialog.getByRole("button", { name: "Confirm promotion", exact: true }).tap();

      await expect
        .poll(async () => (await getCanvas(apiClient, activeCanvas.id))?.scope_kind ?? null)
        .toBe("workspace");
      await expect
        .poll(async () => (await getCanvas(apiClient, activeCanvas.id))?.active_release_id ?? null)
        .toBe(activeReleaseId ?? null);

      await testPage.goto("/");
      await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 20_000 });
      const menuButton = testPage.getByRole("button", { name: "Open menu" });
      await expect(menuButton).toBeVisible();
      await menuButton.tap();

      const workspaceCanvas = testPage.getByTestId(`mobile-workspace-canvas-${activeCanvas.id}`);
      await expect(workspaceCanvas).toBeVisible({ timeout: 15_000 });
      expect((await workspaceCanvas.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await workspaceCanvas.tap();

      await expect(testPage).toHaveURL(new RegExp(`${canvasHref(activeCanvas.id)}$`));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 20_000 });
      await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);
      await expect(testPage.getByTestId("canvas-mobile-actions")).toBeVisible();
      await expect
        .poll(() =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);

      const secondSeeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasIds.push(secondSeeded.canvas.id);
      const secondApproved = await approvePendingCanvasThroughHost(
        testPage,
        apiClient,
        secondSeeded.canvas,
      );
      await promoteCanvas(apiClient, secondApproved);

      await testPage.goto(canvasHref(activeCanvas.id));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 20_000 });
      await expect(testPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 20_000 },
      );
      await expectCanvasFrameFillsHost(testPage);
      await testPage.getByTestId("canvas-mobile-actions").tap();
      const picker = testPage.getByTestId("canvas-mobile-picker");
      await expect(picker).toBeVisible();
      const secondCanvasItem = picker.getByTestId(`canvas-mobile-picker-item-${secondApproved.id}`);
      await expect(secondCanvasItem).toBeVisible();
      expect((await secondCanvasItem.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await secondCanvasItem.tap();

      await expect(testPage).toHaveURL(new RegExp(`${canvasHref(secondApproved.id)}$`));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 20_000 });
      await expect(testPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 20_000 },
      );
      await testPage.getByTestId("canvas-mobile-actions").tap();
      await expect(testPage.getByTestId(`canvas-mobile-picker-item-${canvasIds[0]}`)).toBeVisible();
      await expect(
        testPage.getByRole("button", { name: "Releases and permissions", exact: true }),
      ).toBeVisible();
    } finally {
      await Promise.all(canvasIds.map((canvasId) => removeCanvas(apiClient, canvasId)));
      await releaseFeature();
    }
  });
});
