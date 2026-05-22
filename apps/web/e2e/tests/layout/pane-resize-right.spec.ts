import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  WIDE_VIEWPORT,
  openWideTask,
  dragHorizontalSash,
  expectApproxWidth,
  getColumnSashIndex,
  getDockviewGroupWidth,
  readPinnedDefaultsFromStorage,
} from "../../helpers/dockview-resize";

test.describe("Right pane resize — viewport-proportional cap", () => {
  test("resizes past the old 450px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Right resize past old cap");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -350);
    const width = await getDockviewGroupWidth(testPage, "files");
    expect(width).toBeGreaterThan(600);
  });

  test("respects the viewport-proportional cap (max(800, vw*0.7))", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right cap respect");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    // Drag aggressively left — dockview should clamp at vw*0.7 = 1120.
    await dragHorizontalSash(testPage, sashIdx, -2000);
    const width = await getDockviewGroupWidth(testPage, "files");
    const cap = Math.round(WIDE_VIEWPORT.width * 0.7);
    expect(width).toBeLessThanOrEqual(cap + 10);
  });

  test("user width survives reload (sessionStorage round-trip)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Right resize reload");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -250);
    const before = await getDockviewGroupWidth(testPage, "files");

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(after, before, 10);
  });

  test("user width persists into sessionStorage pinned-defaults slot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right pinned defaults");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -300);
    const live = await getDockviewGroupWidth(testPage, "files");
    const defaults = await readPinnedDefaultsFromStorage(testPage);
    expect(defaults.right).toBeDefined();
    expectApproxWidth(defaults.right ?? 0, live, 12);
  });

  test("new task adopts user's preferred right width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Right propagate A");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -280);
    const widthA = await getDockviewGroupWidth(testPage, "files");
    void session;

    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Right propagate B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${taskB.id}`);
    const sessionB = new SessionPage(testPage);
    await sessionB.waitForLoad();
    await sessionB.waitForDockviewReady();

    const widthB = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(widthB, widthA, 20);
  });

  test("viewport shrink re-clamps an over-cap pinned width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right viewport shrink");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -400);
    const wideWidth = await getDockviewGroupWidth(testPage, "files");
    expect(wideWidth).toBeGreaterThan(700);

    await testPage.setViewportSize({ width: 1100, height: 800 });
    // Allow ResizeObserver tick + applyDynamicConstraints to fire.
    await testPage.waitForTimeout(250);

    const newCap = Math.max(800, Math.round(1100 * 0.7));
    const narrowWidth = await getDockviewGroupWidth(testPage, "files");
    expect(narrowWidth).toBeLessThanOrEqual(newCap + 10);
  });
});
