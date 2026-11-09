// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { watchWs, type WsWatcher } from "../../helpers/causal-waits";
import { firstSessionId } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { expectLiveJobSurvivesSnapshot } from "./background-work-snapshot.helpers";

test("a live background job survives an older empty snapshot on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await expectLiveJobSurvivesSnapshot(testPage, apiClient, seedData);
});

const RUNNING_ID = "bg-running";
const WAITING_ID = "bg-waiting";
const UNKNOWN_ID = "bg-unknown";
const COMPLETED_ID = "bg-completed";
const LONG_COMMAND =
  "npm run build --watch --filter=@kandev/a-very-long-unbroken-package-name-with-no-separators";

async function seedWorkloads(
  apiClient: import("../../helpers/api-client").ApiClient,
  sessionId: string,
  rows: Array<{
    work_id: string;
    title?: string;
    kind?: string;
    state: string;
    exit_code?: number;
  }>,
  ws?: WsWatcher,
) {
  for (const row of rows) {
    const update = ws?.waitForEvent("session.background_work.updated");
    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: row.work_id,
      run_id: row.work_id,
      kind: row.kind ?? "shell",
      title: row.title ?? row.work_id,
      state: row.state,
      exit_code: row.exit_code,
      revision: 10,
    });
    await update;
  }
}

async function openBackgroundWorkFromPanels(page: import("@playwright/test").Page) {
  await page.getByTestId("mobile-session-nav-panels").click();
  await page.getByTestId("mobile-background-work-open").click();
  await expect(page.getByTestId("background-work-mobile-panel")).toBeVisible();
}

test.describe("Background work viewer on a phone", () => {
  test("independent Panels entry reaches history with contained commands", async ({
    apiClient,
    seedData,
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone background history viewer",
      seedData.agentProfileId,
      {
        description: "phone background history",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const sessionId = await firstSessionId(apiClient, task.id);

    await seedWorkloads(
      apiClient,
      sessionId,
      [
        { work_id: UNKNOWN_ID, title: LONG_COMMAND, state: "unknown" },
        { work_id: COMPLETED_ID, title: "Finished job", state: "completed", exit_code: 0 },
      ],
      ws,
    );
    const editor = await session.fillMessage("keep this draft while inspecting history");
    await openBackgroundWorkFromPanels(testPage);

    const overview = testPage.getByTestId("background-work-mobile-overview");
    await expect(overview).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    await expect(testPage.getByTestId(`background-workload-card-${UNKNOWN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-workload-card-${COMPLETED_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);
    await prCapture.screenshot("mobile-background-work-history", {
      caption: "Phone Background Work separates Active and History after Panels opens it",
    });

    // The complete command remains available in the focused detail, with one
    // vertical scroller and no horizontal overflow.
    await testPage.getByTestId(`open-workload-${UNKNOWN_ID}`).click();
    const detail = testPage.getByTestId("background-work-drawer-detail");
    await expect(detail).toBeVisible();
    await expect(testPage.getByTestId("background-work-detail-command")).toContainText(
      LONG_COMMAND,
    );
    const viewport = testPage.viewportSize();
    const detailBox = await detail.boundingBox();
    expect(detailBox).not.toBeNull();
    // The independent Panels entry fills the mobile content area between the
    // fixed top bar and bottom navigation.
    expect(detailBox!.height).toBeGreaterThanOrEqual((viewport?.height ?? 0) * 0.8);

    const overflow = await testPage.evaluate(() => {
      const documentOverflow =
        document.documentElement.scrollWidth - document.documentElement.clientWidth;
      const surface = document.querySelector<HTMLElement>(
        '[data-testid="background-work-mobile-surface"]',
      );
      const surfaceOverflow = surface ? surface.scrollWidth - surface.clientWidth : 0;
      return Math.max(documentOverflow, surfaceOverflow);
    });
    expect(overflow).toBeLessThanOrEqual(0);

    const back = testPage.getByTestId("background-work-drawer-back");
    const backBox = await back.boundingBox();
    expect(backBox!.height).toBeGreaterThanOrEqual(44);
    expect(backBox!.width).toBeGreaterThanOrEqual(44);
    await back.click();
    await expect(overview).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );

    await testPage.getByTestId("background-work-search").fill("missing");
    await expect(testPage.getByTestId("background-work-empty")).toHaveText(
      "No matching background work.",
    );
    await testPage.getByTestId("background-work-search").fill("");

    await testPage.getByTestId("background-work-overview-close").click();
    await expect(testPage.getByTestId("background-work-mobile-panel")).toHaveCount(0);
    await expect(editor).toHaveText("keep this draft while inspecting history");
  });

  test("running chip opens a summary and full-height overview, then dismisses with the last running job", async ({
    apiClient,
    seedData,
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone running background summary",
      seedData.agentProfileId,
      {
        description: "phone running summary",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const sessionId = await firstSessionId(apiClient, task.id);

    await seedWorkloads(
      apiClient,
      sessionId,
      [
        { work_id: RUNNING_ID, title: LONG_COMMAND, state: "running" },
        { work_id: WAITING_ID, title: "Waiting subagent", kind: "subagent", state: "waiting" },
        { work_id: UNKNOWN_ID, title: "Unknown poller", state: "unknown" },
      ],
      ws,
    );

    const editor = await session.fillMessage("draft survives the running summary");
    const chip = testPage.getByTestId("background-work-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await expect(chip).toContainText("1 Background Job");
    await chip.tap();

    const drawer = testPage.getByTestId("background-work-drawer-content");
    await expect(drawer).toBeVisible();
    const summary = drawer.getByTestId("background-work-summary-list");
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId(`background-work-summary-item-${RUNNING_ID}`)).toBeVisible();
    await expect(summary.getByTestId(`background-work-summary-item-${WAITING_ID}`)).toHaveCount(0);
    await expect(summary.getByTestId(`background-work-summary-item-${UNKNOWN_ID}`)).toHaveCount(0);

    await summary.getByTestId("background-work-view-all-button").tap();
    const overview = testPage.getByTestId("background-work-mobile-overview");
    await expect(overview).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-active")).toContainText("Active (2)");
    await expect(testPage.getByTestId("background-work-tab-history")).toContainText("History (1)");
    const drawerBox = await drawer.boundingBox();
    expect(drawerBox).not.toBeNull();
    expect(drawerBox!.height).toBeGreaterThanOrEqual((testPage.viewportSize()?.height ?? 0) * 0.85);
    await prCapture.screenshot("mobile-background-work-overview", {
      caption: "Phone running summary expands to the full-height tabbed overview",
    });

    await testPage.getByTestId("background-work-tab-history").tap();
    await expect(testPage.getByTestId(`background-work-state-${UNKNOWN_ID}`)).toHaveText("Unknown");
    await testPage.getByTestId("background-work-tab-active").tap();
    await expect(testPage.getByTestId(`background-work-state-${WAITING_ID}`)).toHaveText("Waiting");

    // Even at the narrowest supported phone width the list keeps status and
    // Open visible without horizontal scrolling.
    await testPage.setViewportSize({ width: 320, height: 720 });
    await expect(testPage.getByTestId(`open-workload-${RUNNING_ID}`)).toBeVisible();
    const narrowOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(narrowOverflow).toBeLessThanOrEqual(0);

    // The last running job leaves while the overview is open. An overview the
    // user opened is not dismissed by a background update: membership moves to
    // History, the chip disappears, and the draft remains.
    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: RUNNING_ID,
      run_id: RUNNING_ID,
      kind: "shell",
      title: LONG_COMMAND,
      state: "completed",
      exit_code: 0,
      revision: 11,
    });
    await expect(testPage.getByTestId("background-work-tab-active")).toContainText("Active (1)");
    await expect(testPage.getByTestId("background-work-tab-history")).toContainText("History (2)");
    await expect(drawer).toBeVisible();
    await expect(chip).toHaveCount(0);
    await expect(editor).toHaveText("draft survives the running summary");

    await testPage.getByTestId("background-work-overview-close").tap();
    await expect(drawer).toBeHidden();
  });

  test("an open summary dismisses with the last running job and restores composer focus", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone background summary dismissal",
      seedData.agentProfileId,
      {
        description: "phone summary dismissal",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const sessionId = await firstSessionId(apiClient, task.id);

    await seedWorkloads(
      apiClient,
      sessionId,
      [{ work_id: RUNNING_ID, title: LONG_COMMAND, state: "running" }],
      ws,
    );
    const editor = await session.fillMessage("draft survives summary dismissal");
    const chip = testPage.getByTestId("background-work-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await chip.tap();

    const drawer = testPage.getByTestId("background-work-drawer-content");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("background-work-summary-list")).toBeVisible();

    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: RUNNING_ID,
      run_id: RUNNING_ID,
      kind: "shell",
      title: LONG_COMMAND,
      state: "completed",
      exit_code: 0,
      revision: 11,
    });

    await expect(drawer).toBeHidden();
    await expect(chip).toHaveCount(0);
    await expect(editor).toHaveText("draft survives summary dismissal");
    await expect(editor).toBeFocused();
  });
});
