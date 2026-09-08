import type { Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";

/**
 * The automation detail page on a phone.
 *
 * The desktop composition puts the run switcher in a permanent rail resized by
 * dragging its edge — a mouse gesture, and a large share of a 393px viewport.
 * On mobile the same runs move behind a drawer so the transcript and composer,
 * which are the point of the page, get the screen.
 *
 * These tests seed real runs rather than asserting on empty states: a drawer
 * with nothing in it would pass every check below while the feature was
 * completely broken.
 */

type Seed = { workspaceId: string; workflowId: string; startStepId: string };

/** The standing instruction, seeded so the run-detail panel has one to show. */
const STANDING_INSTRUCTION = "Check the overnight drift report and summarise what changed.";

/** An automation with two finished runs, newest last. */
async function seedAutomationWithRuns(apiClient: ApiClient, seed: Seed, name: string) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seed.workspaceId,
    name,
    workflowId: seed.workflowId,
    workflowStepId: seed.startStepId,
    prompt: STANDING_INSTRUCTION,
  });

  const older = await apiClient.createTask(seed.workspaceId, `${name} — older run`, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const newer = await apiClient.createTask(seed.workspaceId, `${name} — newer run`, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  // A session per run, so each has a conversation the rail/drawer can switch to.
  await apiClient.seedAutomationTaskSession(older.id, "WAITING_FOR_INPUT");
  await apiClient.seedAutomationTaskSession(newer.id, "WAITING_FOR_INPUT");
  await apiClient.seedAutomationRun(automation.id, "succeeded", older.id);
  await apiClient.seedAutomationRun(automation.id, "succeeded", newer.id);

  return { automation, older, newer };
}

async function openDetail(testPage: Page, automationId: string) {
  await testPage.goto(`/automations/${automationId}`);
  await expect(testPage.getByTestId("runs-drawer-trigger")).toBeVisible({ timeout: 15_000 });
}

async function seedOpenRun(
  apiClient: import("../helpers/api-client").ApiClient,
  seed: Seed & { agentProfileId: string; repositoryId: string },
  name: string,
) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seed.workspaceId,
    name,
    workflowId: seed.workflowId,
    workflowStepId: seed.startStepId,
    prompt: STANDING_INSTRUCTION,
    agentProfileId: seed.agentProfileId,
  });
  const task = await apiClient.createTaskWithAgent(
    seed.workspaceId,
    `${name} — running task`,
    seed.agentProfileId,
    {
      description: "/sleep 30",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await apiClient.setTaskOrigin(task.id, "automation_run");
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state;
      },
      { timeout: 30_000 },
    )
    .toBe("RUNNING");
  const run = await apiClient.seedAutomationRun(automation.id, "task_created", task.id);
  expect(run.session_id).not.toBe("");
  expect(run.turn_id).not.toBe("");
  return automation;
}

test.describe("Automation detail on mobile", () => {
  test("shows the runs in a drawer rather than a rail", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Sweep");
    await openDetail(testPage, automation.id);

    // The rail is the desktop composition and must not be mounted here.
    await expect(testPage.getByTestId("runs-rail")).toHaveCount(0);
  });

  test("switches to an older run and closes the drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Switch");
    await openDetail(testPage, automation.id);

    await testPage.getByTestId("runs-drawer-trigger").click();
    const completed = testPage.getByTestId("run-group-completed");
    await expect(completed).toBeVisible({ timeout: 10_000 });

    // Second row is the older run; the page opens on the newest.
    const rows = completed.getByRole("button");
    await expect(rows).toHaveCount(2, { timeout: 10_000 });
    await rows.nth(1).click();

    // Selecting must carry the run in the URL and dismiss the drawer — leaving
    // it open would cover the transcript the user just asked for.
    await expect(testPage).toHaveURL(/[?&]run=/, { timeout: 10_000 });
    await expect(testPage.getByTestId("run-group-completed")).toHaveCount(0, { timeout: 10_000 });
  });

  test("keeps the run detail in the drawer, not above the transcript", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Detail");
    await openDetail(testPage, automation.id);
    await expect(testPage.getByTestId("run-transcript")).toBeVisible({ timeout: 15_000 });

    // The standing instruction is the same long text on every run; on a 393px
    // viewport pinning it above the conversation costs most of the screen.
    await expect(testPage.getByTestId("automation-prompt")).toHaveCount(0);
    await expect(testPage.getByTestId("run-detail-toggle")).toHaveCount(0);

    await testPage.getByTestId("runs-drawer-trigger").click();
    const toggle = testPage.getByTestId("run-detail-toggle");
    await expect(toggle).toBeVisible({ timeout: 10_000 });
    await toggle.click();

    const panel = testPage.getByTestId("run-detail-panel");
    await expect(panel.getByTestId("automation-prompt")).toContainText(STANDING_INSTRUCTION);
    // The topbar drops the next-firing note on a phone for want of width, so
    // this is where it said it went.
    await expect(panel.getByTestId("run-detail-next-run")).not.toHaveText("");
  });

  test("offers a composer on a run with a conversation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Reply");
    await openDetail(testPage, automation.id);

    // The reply box is the reason this surface exists; on a phone it must be
    // mounted and on screen rather than pushed below the fold by the switcher.
    //
    // Located by testid, not by role or placeholder text: the composer is a
    // rich editor rather than an <input>, so `getByRole("textbox")` finds
    // nothing, and its placeholder is a decorative paragraph that Playwright
    // does not always resolve.
    const transcript = testPage.getByTestId("run-transcript");
    await expect(transcript).toBeVisible({ timeout: 15_000 });
    const composer = testPage.getByTestId("chat-input-area");
    await expect(composer).toBeVisible({ timeout: 10_000 });

    // On screen, not merely mounted below the fold.
    const box = await composer.boundingBox();
    const height = testPage.viewportSize()?.height ?? 0;
    expect(box, "composer should have a box").not.toBeNull();
    expect(box!.y).toBeLessThan(height);
  });

  test("triggers Run now from the header", async ({ testPage, apiClient, seedData }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Trigger");
    await openDetail(testPage, automation.id);

    const runNow = testPage.getByTestId("automation-run-now");
    await expect(runNow).toBeVisible({ timeout: 10_000 });

    // Reachable, not clipped off the edge by the narrow bar.
    const box = await runNow.boundingBox();
    const width = testPage.viewportSize()?.width ?? 0;
    expect(box, "Run now should have a box").not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);

    await runNow.click();
    // Firing reports what happened — a fire or a skip — rather than going quiet.
    await expect(testPage.getByText(/Triggered|Skipped/)).toBeVisible({ timeout: 15_000 });
  });

  test("stops the selected running run from the drawer header", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await seedOpenRun(apiClient, seedData, "Mobile Exact Stop");
    await openDetail(testPage, automation.id);

    await testPage.getByTestId("runs-drawer-trigger").click();
    const running = testPage.getByTestId("run-group-running");
    await expect(running).toBeVisible({ timeout: 10_000 });
    await running.getByRole("button").first().click();

    const stop = testPage.getByRole("button", { name: "Stop current run" });
    await expect(stop).toBeVisible({ timeout: 10_000 });
    const box = await stop.boundingBox();
    const width = testPage.viewportSize()?.width ?? 0;
    expect(box, "stop action should have a box").not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);

    await stop.click();
    // Selecting a run closes the mobile drawer; the selected-run header is
    // therefore the terminal-state surface after the stop.
    await expect(testPage.getByText("Failed", { exact: true }).first()).toBeVisible({
      timeout: 15_000,
    });
  });

  test("does not scroll sideways with a transcript mounted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedAutomationWithRuns(apiClient, seedData, "Mobile Overflow");
    await openDetail(testPage, automation.id);
    await expect(testPage.getByTestId("run-transcript")).toBeVisible({ timeout: 15_000 });

    // The document itself must not scroll horizontally. Wide content — a
    // transcript's code blocks, a long automation name — belongs in its own
    // scroller, not pushing the page sideways.
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  });
});
