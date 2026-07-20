import { test, expect } from "../../fixtures/test-base";
import { computeRightMaxPx } from "../../../lib/state/layout-manager/caps";
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
    await session.waitForDockviewReady();
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });

    const after = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(after, before, 12);
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
    await session.waitForDockviewReady();
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });

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
    await session.waitForDockviewReady();

    await expect
      .poll(
        async () => {
          return getDockviewGroupWidth(testPage, "files");
        },
        {
          timeout: 5_000,
          message: `Task B right width should be the default (>350), not Task A's narrow ${narrowedA}`,
        },
      )
      .toBeGreaterThan(350);
  });
});
