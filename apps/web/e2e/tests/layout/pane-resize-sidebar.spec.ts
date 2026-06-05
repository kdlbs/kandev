import { test, expect } from "../../fixtures/test-base";
import {
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

test.describe("Sidebar resize — viewport-proportional cap", () => {
  test("resizes past the old 350px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar past cap");
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 480);
    expect(actual).toBeGreaterThan(420);
  });

  test("respects the viewport-proportional cap on narrower screens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar cap narrow", {
      width: 1200,
      height: 800,
    });
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 5000);
    expect(actual).toBeLessThanOrEqual(Math.max(350, Math.round(1200 * 0.3)) + 10);
  });

  test("user width survives reload", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar reload");
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 440);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 12);
  });

  test("toggle sidebar off+on preserves the last user width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar toggle");
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 430);

    // TOGGLE_SIDEBAR shortcut (Ctrl/Cmd+B). Move focus out of any input first.
    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(250);
    await testPage.keyboard.press(`${mod}+b`);
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 20);
  });
});

test.describe("Sidebar width — global across tasks", () => {
  test("a width set in one task applies to a different task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Task A: drag the sidebar to a distinctive width.
    const sessionA = await openWideTask(testPage, apiClient, seedData, "Sidebar global A");
    const widthA = await resizeColumnViaSplitview(testPage, "sidebar", 440);

    // Task B is a separate task (separate env). It must open at task A's width
    // because the left sidebar width is now a single global preference.
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar global B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${taskB.id}`);
    await sessionA.waitForLoad();
    await sessionA.waitForDockviewReady();

    const widthB = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(widthB, widthA, 12);
  });

  test("clamps to fit a smaller screen, then restores on a wider one", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Set a wide sidebar on a 1600px screen.
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar clamp");
    const wide = await resizeColumnViaSplitview(testPage, "sidebar", 520);
    expect(wide).toBeGreaterThan(450);

    // Shrink the viewport. The sidebar must settle within the smaller screen's
    // cap (max(350, 30% of 1100) = 350, bounded by viewport-300) — not stay at
    // 520 and not drift.
    await testPage.setViewportSize({ width: 1100, height: 800 });
    await session.waitForDockviewReady();
    await testPage.waitForTimeout(400);
    const small = await getDockviewGroupWidth(testPage, "sidebar");
    const smallCap = Math.max(350, Math.round(1100 * 0.3));
    expect(small).toBeLessThanOrEqual(smallCap + 12);

    // Back to a wide screen: the raw 520 preference is restored (clamp does not
    // overwrite stored width).
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await session.waitForDockviewReady();
    await testPage.waitForTimeout(400);
    const restored = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(restored, wide, 16);
  });

  test("Default layout preset resets the custom width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar reset");
    const custom = await resizeColumnViaSplitview(testPage, "sidebar", 480);
    expect(custom).toBeGreaterThan(420);

    // Apply the "Default" layout preset (resetWidths=true).
    await testPage.getByTestId("layout-preset-trigger").click();
    await testPage.locator('[data-testid="layout-preset-item"][data-preset-id="default"]').click();
    await session.waitForDockviewReady();
    await testPage.waitForTimeout(400);

    // Sidebar returns to the ratio default (1600*0.25=400, clamped to 350).
    const afterReset = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(afterReset, 350, 16);
  });
});
