import { test, expect } from "../../fixtures/test-base";
import { computeRightMaxPx } from "../../../lib/state/layout-manager/caps";
import { TERMINAL_DEFAULT_ID } from "../../../lib/state/layout-manager/constants";
import {
  WIDE_VIEWPORT,
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Right pane resize — container-proportional cap", () => {
  test("resizes past the old 450px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Right resize past old cap");
    const actual = await resizeColumnViaSplitview(testPage, "right", 700);
    expect(actual).toBeGreaterThan(600);
  });

  test("respects the container cap and center comfort width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right cap respect");
    const actual = await resizeColumnViaSplitview(testPage, "right", 5000);
    const dockviewBox = await testPage.locator(".dv-dockview").boundingBox();
    expect(dockviewBox).not.toBeNull();
    const availableWidth = dockviewBox?.width ?? 0;
    const sidebarWidth = await getDockviewGroupWidth(testPage, "sidebar").catch(() => 0);
    const cap = computeRightMaxPx(availableWidth, sidebarWidth);
    expect(actual).toBeLessThanOrEqual(cap + 10);
  });

  test("user width survives reload (localStorage dockview-layout-v3 round-trip)", async ({
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

  test("initial restore reconciles a stale saved grid width without a manual resize", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 2200, height: 900 });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Right stale grid restore",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("created task did not return a session_id");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    expect(await resizeColumnViaSplitview(testPage, "right", 900)).toBeGreaterThan(800);

    const staleLayout = await testPage.evaluate((sessionId) => {
      type StoreWindow = Window & {
        __KANDEV_E2E_STORE__?: {
          getState: () => { environmentIdBySessionId: Record<string, string> };
        };
      };
      const store = (window as StoreWindow).__KANDEV_E2E_STORE__;
      const envId = store?.getState().environmentIdBySessionId[sessionId];
      if (!envId) throw new Error("environment id not hydrated");
      const key = `kandev.dockview.env-layout-v3.${envId}`;
      const raw = window.sessionStorage.getItem(key);
      if (!raw) throw new Error("env layout was not persisted");
      const layout = JSON.parse(raw);
      const columns = layout.grid?.root?.data;
      if (!Array.isArray(columns) || columns.length < 2) {
        throw new Error("saved layout does not contain center and right columns");
      }
      layout.grid.width = 1900;
      columns[0].size = 1000;
      columns[columns.length - 1].size = 900;
      return { key, json: JSON.stringify(layout) };
    }, task.session_id);

    await testPage.addInitScript(({ key, json }) => {
      window.sessionStorage.setItem(key, json);
    }, staleLayout);
    await testPage.setViewportSize({ width: 1540, height: 900 });
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const dockviewBox = await testPage.locator(".dv-dockview").boundingBox();
    expect(dockviewBox).not.toBeNull();
    const rightWidth = await getDockviewGroupWidth(testPage, "files");
    const terminalWidth = await getDockviewGroupWidth(testPage, TERMINAL_DEFAULT_ID);
    const centerWidth = await getDockviewGroupWidth(testPage, `session:${task.session_id}`);
    const cap = computeRightMaxPx(dockviewBox?.width ?? 0);
    expect(rightWidth).toBeLessThanOrEqual(cap + 10);
    expectApproxWidth(terminalWidth, rightWidth, 10);
    expect(centerWidth).toBeGreaterThanOrEqual(478);
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
    // Allow the container ResizeObserver and pinned-target enforcement to
    // settle. Do not call the resize helper here: it reapplies constraints and
    // would mask a failure to update them automatically on viewport changes.
    await testPage.waitForTimeout(500);

    const dockviewBox = await testPage.locator(".dv-dockview").boundingBox();
    expect(dockviewBox).not.toBeNull();
    const narrowWidth = await getDockviewGroupWidth(testPage, "files");
    const sidebarWidth = await getDockviewGroupWidth(testPage, "sidebar").catch(() => 0);
    const newCap = computeRightMaxPx(dockviewBox?.width ?? 0, sidebarWidth);
    expect(narrowWidth).toBeLessThanOrEqual(newCap + 10);
  });
});

test.describe("Right pane width — per-task isolation", () => {
  test("a narrow resize in Task A does not leak into Task B", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize(WIDE_VIEWPORT);

    // Two tasks, same default layout. Each gets its own env id and its own
    // persisted dockview layout in sessionStorage.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Right Width Task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Right Width Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Right Width Task A").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();

    // Resize Task A's right column to a deliberately narrow width. The default
    // is ~450 on a 1600px viewport, so 240 is unambiguously a user override.
    const narrowedA = await resizeColumnViaSplitview(testPage, "right", 240);
    expect(narrowedA).toBeLessThan(300);

    // Switch to Task B (no prior resize, default width). Regression: a stale
    // global "right" pinned target from Task A's drag used to leak through
    // captureRightTarget / enforcePinnedTargets after fromJSON, snapping
    // Task B's right column to Task A's narrow width — and the next
    // debounced persist would overwrite Task B's saved layout with the
    // narrow width, making the leak sticky.
    await session.clickTaskInSidebar("Right Width Task B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), {
      timeout: 15_000,
    });
    await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 15_000 });
    // Allow fromJSON + fixups-capture + enforcePinnedTargets to settle.
    await testPage.waitForTimeout(500);

    const widthB = await getDockviewGroupWidth(testPage, "files");
    expect(
      widthB,
      `Task B right width ${widthB} should be the default (>350), not Task A's narrow ${narrowedA}`,
    ).toBeGreaterThan(350);
  });
});
