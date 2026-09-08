import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

const PLAN_MARKER = "Plan table resize marker";
const PLAN_MARKDOWN = [
  "## Resizable table",
  "",
  "| Setting | Effect | Notes |",
  "| --- | --- | --- |",
  `| strictDepBuilds | Blocks unapproved scripts | ${PLAN_MARKER} |`,
  "| allowBuilds | Allows approved scripts | Ephemeral width |",
].join("\n");

async function openPlanPanel(session: SessionPage) {
  if (await session.planPanel.isVisible()) return;
  await session.togglePlanMode();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

async function seedPlanTable(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Resizable Plan table",
    seedData.agentProfileId,
    {
      description: planScript(PLAN_MARKDOWN),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  const initialPlan = await apiClient.getTaskPlan(task.id);
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.composerReady();
  await openPlanPanel(session);
  await expect(session.planPanel).toContainText(PLAN_MARKER, { timeout: 15_000 });
  if (!initialPlan) throw new Error("seeded plan was not available");
  return { initialPlan: initialPlan.content, session, taskId: task.id };
}

test.describe("Plan table column resizing", () => {
  test("resizes an internal boundary without persisting the width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Covers AC-UI-RESIZABLE-MARKDOWN-TABLES-001.10 and .11.
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1100, height: 800 });
    const { initialPlan, session, taskId } = await seedPlanTable(testPage, apiClient, seedData);

    const editor = session.planEditor();
    const table = editor.locator("table", { hasText: PLAN_MARKER });
    const wrapper = table.locator("xpath=..");
    const firstHeader = table.locator("th").first();
    await expect(table).toBeVisible();

    const initialHeaderBox = await firstHeader.boundingBox();
    expect(initialHeaderBox).not.toBeNull();
    const boundaryY = initialHeaderBox!.y + initialHeaderBox!.height / 2;
    await testPage.mouse.move(initialHeaderBox!.x + initialHeaderBox!.width - 2, boundaryY);

    const handle = editor.locator(".column-resize-handle").first();
    await expect(handle).toBeVisible();
    const initialWidth = initialHeaderBox!.width;
    const boundaryX = initialHeaderBox!.x + initialHeaderBox!.width - 2;
    await testPage.mouse.down();
    await testPage.mouse.move(boundaryX + 60, boundaryY);
    await testPage.mouse.up();

    const grownWidth = await firstHeader.evaluate(
      (element) => element.getBoundingClientRect().width,
    );
    expect(grownWidth - initialWidth).toBeCloseTo(60, 0);

    const grownHeaderBox = await firstHeader.boundingBox();
    expect(grownHeaderBox).not.toBeNull();
    const grownBoundaryX = grownHeaderBox!.x + grownHeaderBox!.width - 2;
    const grownBoundaryY = grownHeaderBox!.y + grownHeaderBox!.height / 2;
    await testPage.mouse.move(grownBoundaryX, grownBoundaryY);
    await testPage.mouse.down();
    await testPage.mouse.move(grownBoundaryX - 1000, grownBoundaryY);
    await testPage.mouse.up();
    expect(
      await firstHeader.evaluate((element) => element.getBoundingClientRect().width),
    ).toBeCloseTo(64, 0);

    expect(
      await wrapper.evaluate((element) =>
        ["auto", "scroll"].includes(getComputedStyle(element).overflowX),
      ),
    ).toBe(true);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
    // Stay beyond the Plan panel's 1.5-second autosave window. A resize-only
    // transaction must not enqueue a later canonicalized Markdown save.
    await dwell(
      testPage,
      2_500,
      "negative-assertion",
      "a resize-only transaction must never trigger the 1.5-second Plan autosave, so there is no save event to await",
    );
    expect((await apiClient.getTaskPlan(taskId))?.content).toBe(initialPlan);

    await testPage.reload();
    await session.waitForLoad();
    await openPlanPanel(session);
    const reloadedFirstHeader = session.planEditor().locator("table th").first();
    await expect(reloadedFirstHeader).toBeVisible({ timeout: 15_000 });
    expect(
      await reloadedFirstHeader.evaluate((element) => element.getBoundingClientRect().width),
    ).toBeGreaterThan(64);
    expect((await apiClient.getTaskPlan(taskId))?.content).toBe(initialPlan);
  });
});
