import { expect, test } from "../../fixtures/test-base";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import {
  enableCanvasFeature,
  expectCanvasFrameFillsHost,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test.describe("Plugin-backed canvases in the desktop task workbench", () => {
  test("discovers, reviews, and operates the first task canvas from the workbench", async ({
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
      await expect(testPage.getByTestId("canvas-host-state")).toHaveText(
        "Permission review required",
      );
      await testPage.getByRole("button", { name: "Releases and permissions", exact: true }).click();

      const releasesDialog = testPage.getByTestId("canvas-releases-dialog");
      await expect(releasesDialog).toBeVisible();
      await expect(
        releasesDialog.getByTestId(
          `canvas-release-permissions-${seeded.canvas.pending_release?.id}`,
        ),
      ).toBeVisible();
      await releasesDialog.getByRole("button", { name: "Approve release", exact: true }).click();
      const closeReleasesDialog = releasesDialog
        .locator('[data-slot="dialog-footer"]')
        .getByRole("button", { name: "Close", exact: true });
      await expect(closeReleasesDialog).toBeVisible();
      await closeReleasesDialog.click();

      await expect(testPage.getByTestId("canvas-host-state")).toHaveText("Ready", {
        timeout: 20_000,
      });
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
});
