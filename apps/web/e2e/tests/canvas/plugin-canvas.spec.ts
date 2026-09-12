import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import { SessionPage } from "../../pages/session-page";
import {
  canvasHref,
  enableCanvasFeature,
  expectCanvasFrameFillsHost,
  listCanvasReleases,
  removeCanvas,
  seedTaskCanvas,
  waitForSessionWorkspace,
  writeCanvasSource,
} from "./canvas-fixture";

test.describe("Plugin-backed canvases in the desktop task workbench", () => {
  test("shows the canvas creation prompt and retains the edited description on desktop", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let taskId: string | undefined;
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

      await testPage.goto(
        `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/canvases`,
      );
      await expect(testPage.getByTestId("workspace-canvases-page")).toBeVisible();
      await testPage.getByTestId("settings-create-canvas").click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("source-mode-scratch")).toHaveAttribute(
        "aria-checked",
        "true",
      );
      await expect(dialog.getByTestId("executor-profile-selector")).toContainText(
        localProfile!.name,
      );

      const defaultPrompt = await dialog.getByTestId("task-description-input").inputValue();
      for (const tool of [
        "create_canvas_kandev",
        "read_canvas_authoring_skill_kandev",
        "publish_canvas_kandev",
      ]) {
        expect(defaultPrompt, `desktop preset is missing ${tool}`).toContain(tool);
      }
      expect(defaultPrompt).not.toContain("e2e:mcp:");

      const editedDescription = "desktop canvas prompt override";
      await dialog.getByTestId("task-title-input").fill("E2E Desktop Canvas Task");
      await dialog.getByTestId("task-description-input").fill(editedDescription);

      const startAgent = dialog.getByTestId("submit-start-agent");
      await expect(startAgent).toBeEnabled();
      const responsePromise = waitForHttp(testPage, "POST", /\/api\/v1\/tasks$/);
      await startAgent.click();
      const response = await responsePromise;
      const responseBody = await response.text();
      expect(response.status(), responseBody).toBe(200);
      const created = JSON.parse(responseBody) as { id: string };
      taskId = created.id;
      expect(taskId).toBeTruthy();

      await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}(?:[?]|$)`));
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect
        .poll(async () => (await apiClient.getTask(taskId!)).description)
        .toBe(editedDescription);
      await expect
        .poll(() =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);
    } finally {
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await releaseFeature();
    }
  });

  test("discovers and operates an owner-created task canvas from the workbench", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;

      await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 20_000 });
      await expect
        .poll(
          () =>
            testPage.evaluate((id) => {
              const dockview = (
                window as unknown as {
                  __dockviewApi__?: { panels?: Array<{ id: string }> };
                }
              ).__dockviewApi__;
              return dockview?.panels?.filter((panel) => panel.id === `canvas:${id}`).length ?? 0;
            }, seeded.canvas.id),
          { timeout: 20_000 },
        )
        .toBe(1);
      await expect(testPage.getByTestId("canvas-host-state")).toHaveText("Ready", {
        timeout: 20_000,
      });
      expect(seeded.canvas.pending_release).toBeUndefined();
      expect(seeded.canvas.active_release_status).toBe("valid");
      await expect(testPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 20_000 },
      );
      await expectCanvasFrameFillsHost(testPage);

      const canvasPanelId = `canvas:${seeded.canvas.id}`;
      const normalCanvasWidth = await testPage.evaluate((id) => {
        const api = (
          window as unknown as {
            __dockviewApi__?: {
              getPanel: (panelId: string) => { group: { width: number } } | undefined;
            };
          }
        ).__dockviewApi__;
        const panel = api?.getPanel(id);
        if (!panel) throw new Error("canvas panel not found");
        return panel.group.width;
      }, canvasPanelId);
      await testPage.evaluate((id) => {
        const api = (
          window as unknown as {
            __dockviewApi__?: {
              getPanel: (
                panelId: string,
              ) => { group: { api: { maximize: () => void } } } | undefined;
            };
          }
        ).__dockviewApi__;
        api?.getPanel(id)?.group.api.maximize();
      }, canvasPanelId);
      await expect
        .poll(() =>
          testPage.evaluate(() => {
            const api = (
              window as unknown as { __dockviewApi__?: { hasMaximizedGroup: () => boolean } }
            ).__dockviewApi__;
            return api?.hasMaximizedGroup() ?? false;
          }),
        )
        .toBe(true);
      await expectCanvasFrameFillsHost(testPage);
      await testPage.evaluate((id) => {
        const api = (
          window as unknown as {
            __dockviewApi__?: {
              getPanel: (
                panelId: string,
              ) => { group: { api: { exitMaximized: () => void } } } | undefined;
            };
          }
        ).__dockviewApi__;
        api?.getPanel(id)?.group.api.exitMaximized();
      }, canvasPanelId);
      await expect
        .poll(() =>
          testPage.evaluate(() => {
            const api = (
              window as unknown as { __dockviewApi__?: { hasMaximizedGroup: () => boolean } }
            ).__dockviewApi__;
            return api?.hasMaximizedGroup() ?? false;
          }),
        )
        .toBe(false);
      await expectCanvasFrameFillsHost(testPage);

      await resizeColumnViaSplitview(testPage, "right", 480);
      await expect
        .poll(() =>
          testPage.evaluate(
            ({ id, previous }) => {
              const api = (
                window as unknown as {
                  __dockviewApi__?: {
                    getPanel: (panelId: string) => { group: { width: number } } | undefined;
                  };
                }
              ).__dockviewApi__;
              const width = api?.getPanel(id)?.group.width;
              return typeof width === "number" && Math.abs(width - previous) > 2;
            },
            { id: canvasPanelId, previous: normalCanvasWidth },
          ),
        )
        .toBe(true);
      await expectCanvasFrameFillsHost(testPage);
      const fixture = testPage.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(fixture.getByTestId("canvas-fixture-script")).toHaveText("inline-ready");
      await expect(fixture.getByTestId("canvas-fixture-appearance-mode")).toHaveText("light");
      await expect(fixture.getByTestId("canvas-fixture-appearance-color-scheme")).toHaveText(
        "light",
      );
      const lightBackground = await fixture
        .getByTestId("canvas-fixture-appearance-background")
        .textContent();
      await expect(fixture.getByTestId("canvas-fixture-context")).toHaveText(seeded.taskId);
      await expect(fixture.getByTestId("canvas-fixture-task-count")).toHaveText("1");
      await expect(fixture.getByTestId("canvas-fixture-workflow-count")).toHaveText("1");
      await expect(fixture.getByTestId("canvas-fixture-step-id")).not.toHaveText("loading");
      await expect(fixture.getByTestId("canvas-fixture-sse-status")).toHaveText("connected");

      await fixture.getByTestId("canvas-fixture-move").dispatchEvent("click");
      await expect(fixture.getByTestId("canvas-fixture-move-status")).toHaveText(/moved:/);

      await fixture.getByTestId("canvas-fixture-continue").dispatchEvent("click");
      await expect(fixture.getByTestId("canvas-fixture-message-status")).toHaveText("accepted");
      await expect
        .poll(async () =>
          Number(await fixture.getByTestId("canvas-fixture-sse-events").textContent()),
        )
        .toBeGreaterThan(0);

      await fixture.getByTestId("canvas-fixture-state").dispatchEvent("click");
      await expect(fixture.getByTestId("canvas-fixture-state-status")).toHaveText(
        /conflict-recovered:/,
      );

      await fixture.getByTestId("canvas-fixture-reconnect").dispatchEvent("click");
      await expect(fixture.getByTestId("canvas-fixture-sse-status")).toHaveText("connected");
      await fixture.getByTestId("canvas-fixture-resync").dispatchEvent("click");
      await expect(fixture.getByTestId("canvas-fixture-sse-resync")).toHaveText("received");

      await expect
        .poll(
          () =>
            testPage.evaluate((id) => {
              const dockview = (
                window as unknown as {
                  __dockviewApi__?: {
                    panels?: Array<{
                      id: string;
                      api?: { component?: string };
                      params?: Record<string, unknown>;
                    }>;
                  };
                }
              ).__dockviewApi__;
              const panel = dockview?.panels?.find((candidate) => candidate.id === `canvas:${id}`);
              return panel
                ? {
                    id: panel.id,
                    component: panel.api?.component,
                    canvasId: panel.params?.canvasId,
                  }
                : null;
            }, seeded.canvas.id),
          { timeout: 10_000 },
        )
        .toEqual({
          id: `canvas:${seeded.canvas.id}`,
          component: "canvas",
          canvasId: seeded.canvas.id,
        });

      const themeToggle = testPage.getByRole("button", {
        name: "Switch to Dark Mode",
        exact: true,
      });
      await expect(themeToggle).toBeVisible();
      await themeToggle.evaluate((element) => (element as HTMLButtonElement).click());
      await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);
      await expect(fixture.getByTestId("canvas-fixture-appearance-mode")).toHaveText("dark");
      await expect(fixture.getByTestId("canvas-fixture-appearance-color-scheme")).toHaveText(
        "dark",
      );
      await expect
        .poll(() => fixture.getByTestId("canvas-fixture-appearance-background").textContent())
        .not.toBe(lightBackground);
    } finally {
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("shows recoverable startup failure and retries with a fresh runtime", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let contextFailures = 0;
    await testPage.route("**/_kandev/v1/context", async (route) => {
      if (contextFailures === 0) {
        contextFailures += 1;
        await route.abort();
        return;
      }
      await route.continue();
    });
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;

      await expect(testPage.getByTestId("canvas-host-state")).toHaveText("Canvas unavailable", {
        timeout: 20_000,
      });
      await expect(testPage.getByTestId("web-app-frame")).toHaveCount(0);
      await expect(testPage.getByRole("button", { name: "Try again", exact: true })).toBeVisible();

      await testPage.getByRole("button", { name: "Try again", exact: true }).click();
      await expect(testPage.getByTestId("canvas-host-state")).toHaveText("Ready", {
        timeout: 20_000,
      });
      await expect(testPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 20_000 },
      );
      expect(contextFailures).toBe(1);
    } finally {
      await testPage.unroute("**/_kandev/v1/context");
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("keeps the desktop release review wide without scrolling two permissions", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 1280, height: 720 });

    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true, {
        noPermissions: true,
      });
      canvasId = seeded.canvas.id;

      const workspacePath = await waitForSessionWorkspace(
        apiClient,
        seeded.taskId,
        seeded.taskSessionId,
      );
      writeCanvasSource(workspacePath, seeded.canvas, { minimalPermissions: true });
      const publishScript = `e2e:mcp:kandev:publish_canvas_kandev(${JSON.stringify({
        canvas_id: seeded.canvas.id,
        source_path: `.kandev/canvases/${seeded.canvas.id}`,
      })})`;
      const reviewSession = await apiClient.launchSession({
        task_id: seeded.taskId,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: `e2e:delay(2500)\n${publishScript}`,
      });
      const reviewWorkspacePath = await waitForSessionWorkspace(
        apiClient,
        seeded.taskId,
        reviewSession.session_id,
      );
      writeCanvasSource(reviewWorkspacePath, seeded.canvas, { minimalPermissions: true });
      await waitForSessionDone(
        apiClient,
        seeded.taskId,
        reviewSession.session_id,
        "The permission-increasing canvas publication did not finish.",
        45_000,
      );
      let pendingReleaseId: string | undefined;
      await expect
        .poll(
          async () => {
            const releases = await listCanvasReleases(apiClient, canvasId!);
            pendingReleaseId = releases.find(
              (release) => release.validation_status === "pending_permission",
            )?.id;
            return pendingReleaseId ?? null;
          },
          {
            timeout: 30_000,
            message: "The permission-increasing canvas release did not pend.",
          },
        )
        .not.toBeNull();
      expect(pendingReleaseId).toBeTruthy();

      await testPage.goto(canvasHref(canvasId));
      const releasesButton = testPage.getByRole("button", {
        name: "Releases and permissions",
        exact: true,
      });
      await expect(releasesButton).toBeVisible({
        timeout: 20_000,
      });
      await releasesButton.click();

      const dialog = testPage.getByTestId("canvas-releases-dialog");
      await expect(dialog).toBeVisible();
      const dialogBox = await dialog.boundingBox();
      expect(dialogBox?.width).toBeGreaterThanOrEqual(720);
      expect(dialogBox?.width).toBeLessThanOrEqual(780);

      const permissions = dialog.getByTestId("canvas-permission-summary");
      await expect(permissions).toBeVisible();
      await expect(permissions.locator("li")).toHaveCount(2);
      const scrollMetrics = await dialog
        .getByTestId("canvas-release-review-scroll")
        .evaluate((element) => ({
          clientHeight: element.clientHeight,
          scrollHeight: element.scrollHeight,
        }));
      expect(scrollMetrics.scrollHeight).toBeLessThanOrEqual(scrollMetrics.clientHeight);
    } finally {
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });
});
