import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  WIDE_VIEWPORT,
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  readPinnedDefaultsFromStorage,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

test.describe("Right pane resize — viewport-proportional cap", () => {
  test("resizes past the old 450px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Right resize past old cap");
    const actual = await resizeColumnViaSplitview(testPage, "right", 700);
    expect(actual).toBeGreaterThan(600);
  });

  test("respects the viewport-proportional cap (max(800, vw*0.7))", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right cap respect");
    const actual = await resizeColumnViaSplitview(testPage, "right", 5000);
    const cap = Math.round(WIDE_VIEWPORT.width * 0.7);
    expect(actual).toBeLessThanOrEqual(cap + 10);
  });

  test("user width survives reload (sessionStorage round-trip)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Right resize reload");
    const before = await resizeColumnViaSplitview(testPage, "right", 600);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(after, before, 12);
  });

  test("user width persists into sessionStorage pinned-defaults slot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right pinned defaults");
    const live = await resizeColumnViaSplitview(testPage, "right", 650);
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
    const widthA = await resizeColumnViaSplitview(testPage, "right", 620);
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
    const wideWidth = await resizeColumnViaSplitview(testPage, "right", 900);
    expect(wideWidth).toBeGreaterThan(700);

    await testPage.setViewportSize({ width: 1100, height: 800 });
    // Allow ResizeObserver tick + applyDynamicConstraints to fire, then attempt
    // a re-resize that would exceed the new cap.
    await testPage.waitForTimeout(300);
    const narrowWidth = await resizeColumnViaSplitview(testPage, "right", 1500);

    const newCap = Math.max(800, Math.round(1100 * 0.7));
    expect(narrowWidth).toBeLessThanOrEqual(newCap + 10);
  });
});
