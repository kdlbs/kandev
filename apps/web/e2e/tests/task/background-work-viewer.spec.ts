import { expect, test } from "../../fixtures/test-base";
import { watchWs, type WsWatcher } from "../../helpers/causal-waits";
import { firstSessionId } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { expectLiveJobSurvivesSnapshot } from "./background-work-snapshot.helpers";

test("a live background job survives an older empty snapshot", async ({
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
const LONG_DETAIL_COMMAND =
  Array.from({ length: 40 }, (_, index) => `echo line-${index}-${"x".repeat(80)}`).join("\n") +
  "\nCOMMAND-END-MARKER";

async function openBackgroundWorkFromPanelMenu(page: import("@playwright/test").Page) {
  await page.getByTestId("dockview-add-panel-btn").first().click();
  await page.getByTestId("add-panel-background-work").click();
  await expect(page.getByTestId("background-work-overview")).toBeVisible();
}

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

test.describe("Background work viewer", () => {
  test("running-only summary and Active/History separation", async ({
    apiClient,
    seedData,
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Background work viewer",
      seedData.agentProfileId,
      {
        description: "background work viewer",
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
        { work_id: COMPLETED_ID, title: "Finished job", state: "completed", exit_code: 0 },
      ],
      ws,
    );

    const chip = testPage.getByTestId("background-work-chip");
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("1 Background Job");

    await chip.click();
    const summary = testPage.getByTestId("background-work-summary-list");
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId(`background-work-summary-item-${RUNNING_ID}`)).toBeVisible();
    await expect(summary.getByTestId(`background-work-summary-item-${WAITING_ID}`)).toHaveCount(0);
    await expect(summary.getByTestId(`background-work-summary-item-${UNKNOWN_ID}`)).toHaveCount(0);
    await expect(summary).not.toContainText("waiting");
    await expect(summary).not.toContainText("unknown");

    await summary.getByTestId("background-work-view-all-button").click();
    const overview = testPage.getByTestId("background-work-overview");
    await expect(overview).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-active")).toContainText("Active (2)");
    await expect(testPage.getByTestId("background-work-tab-history")).toContainText("History (2)");
    await expect(testPage.getByTestId(`background-workload-card-${RUNNING_ID}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-workload-card-${WAITING_ID}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-workload-card-${UNKNOWN_ID}`)).toHaveCount(0);

    await testPage.getByTestId("background-work-tab-history").click();
    await expect(testPage.getByTestId(`background-workload-card-${UNKNOWN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-work-state-${UNKNOWN_ID}`)).toHaveText("Unknown");
    await expect(testPage.getByTestId(`background-work-state-${COMPLETED_ID}`)).toHaveText(
      "Completed",
    );
    await prCapture.screenshot("background-work-viewer-history", {
      caption: "Background Work shows Active and History with running-only chip summaries",
    });

    // Counts stay independent from the query, and an empty search keeps the
    // selected History tab instead of resetting to Active.
    await testPage.getByTestId("background-work-search").fill("missing");
    await expect(testPage.getByTestId("background-work-empty")).toHaveText(
      "No matching background work.",
    );
    await expect(testPage.getByTestId("background-work-tab-history")).toContainText("History (2)");
    await testPage.getByTestId("background-work-search").fill("");

    // A live transition never closes the open overview or changes its tab.
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
    await expect(testPage.getByTestId("background-work-tab-history")).toContainText("History (3)");
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    await expect(chip).toHaveCount(0);
  });

  test("independent entry reaches history without a chip and contains long commands", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Background work history entry",
      seedData.agentProfileId,
      {
        description: "background work history",
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

    await openBackgroundWorkFromPanelMenu(testPage);
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    await expect(testPage.getByTestId(`background-workload-card-${UNKNOWN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-workload-card-${COMPLETED_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);

    await testPage.getByTestId(`open-workload-${UNKNOWN_ID}`).click();
    const detail = testPage.getByTestId(`background-work-detail-${UNKNOWN_ID}`);
    await expect(detail).toBeVisible();
    await expect(testPage.getByTestId("background-work-detail-command")).toContainText(
      LONG_COMMAND,
    );

    const overflow = await testPage.evaluate(() => {
      const documentOverflow =
        document.documentElement.scrollWidth - document.documentElement.clientWidth;
      const panel = document.querySelector<HTMLElement>('[data-testid^="background-work-detail-"]');
      const panelOverflow = panel ? panel.scrollWidth - panel.clientWidth : 0;
      return Math.max(documentOverflow, panelOverflow);
    });
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("narrow fine-pointer viewport keeps touch-sized running inspection", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 320, height: 720 });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Narrow fine-pointer background work",
      seedData.agentProfileId,
      {
        description: "narrow fine-pointer background work",
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

    const chip = testPage.getByTestId("background-work-chip");
    await expect(chip).toBeVisible();
    const chipBox = await chip.boundingBox();
    expect(chipBox).not.toBeNull();
    expect(chipBox!.height).toBeGreaterThanOrEqual(44);
    expect(chipBox!.width).toBeGreaterThanOrEqual(44);

    await chip.click();
    const summary = testPage.getByTestId("background-work-summary-list");
    await expect(summary).toBeVisible();
    const open = summary.getByTestId(`background-work-open-button-${RUNNING_ID}`);
    const openBox = await open.boundingBox();
    expect(openBox).not.toBeNull();
    expect(openBox!.height).toBeGreaterThanOrEqual(44);
    expect(openBox!.width).toBeGreaterThanOrEqual(44);

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("keeps a command taller than the detail viewport reachable while scrolling", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Tall background command",
      seedData.agentProfileId,
      {
        description: "tall background command",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const sessionId = await firstSessionId(apiClient, task.id);

    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: UNKNOWN_ID,
      run_id: UNKNOWN_ID,
      kind: "shell",
      title: LONG_DETAIL_COMMAND,
      state: "unknown",
      output: "OUTPUT-END-MARKER",
      revision: 10,
    });

    await openBackgroundWorkFromPanelMenu(testPage);
    await testPage.getByTestId(`open-workload-${UNKNOWN_ID}`).click();
    const detail = testPage.getByTestId(`background-work-detail-${UNKNOWN_ID}`);
    await expect(detail).toBeVisible();
    const scroll = testPage.getByTestId("background-work-detail-scroll");
    const metrics = await scroll.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);

    const command = testPage.getByTestId("background-work-detail-command");
    await expect(command).toContainText("COMMAND-END-MARKER");
    await command.evaluate((element) => element.scrollIntoView({ block: "end" }));
    const [commandBox, scrollBox] = await Promise.all([
      command.boundingBox(),
      scroll.boundingBox(),
    ]);
    expect(commandBox).not.toBeNull();
    expect(scrollBox).not.toBeNull();
    expect(commandBox!.y + commandBox!.height).toBeLessThanOrEqual(
      scrollBox!.y + scrollBox!.height + 1,
    );
    expect(commandBox!.y + commandBox!.height).toBeGreaterThanOrEqual(scrollBox!.y);

    await expect(detail.getByTestId(`background-work-state-${UNKNOWN_ID}`)).toBeVisible();
    await expect(detail.getByTestId("background-work-actions-unavailable")).toBeVisible();

    await scroll.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(scroll.getByText(/OUTPUT-END-MARKER/)).toBeVisible();

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
