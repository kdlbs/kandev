import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  attachGatewayTrafficCapture,
  summarizeGatewayTraffic,
  type GatewayTrafficFrame,
} from "../../helpers/ws-traffic";

const TASK_COUNT = 27;
const SWITCH_COUNT = 5;
const MAX_SUBSCRIPTION_OPERATIONS = SWITCH_COUNT * 3;

const SESSION_DETAIL_ACTIONS = new Set([
  "session.message.added",
  "session.message.updated",
  "session.message.deleted",
  "session.git.event",
  "session.shell.output",
  "session.process.output",
  "session.process.status",
  "session.available_commands",
  "session.mode_changed",
  "session.agent_capabilities",
  "session.models_updated",
  "session.mcp_status_updated",
  "session.info_updated",
  "session.todos_updated",
  "session.prompt_usage",
  "session.workspace_sources.updated",
  "workspace.file.changes",
]);

type SeededTask = {
  id: string;
  title: string;
  sessionId: string;
};

function trafficTaskTitle(index: number): string {
  return `Traffic Task ${String(index + 1).padStart(2, "0")}`;
}

function sessionIdFromFrame(frame: GatewayTrafficFrame): string | undefined {
  return frame.sessionId;
}

test.describe("Session stream budget", () => {
  test("keeps task-switcher traffic bounded with 27 task sessions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const capture = attachGatewayTrafficCapture(testPage);
    const trafficStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Traffic measurement",
      1,
    );
    const tasks = await Promise.all(
      Array.from({ length: TASK_COUNT }, (_, index) =>
        apiClient.createTask(seedData.workspaceId, trafficTaskTitle(index), {
          workflow_id: seedData.workflowId,
          workflow_step_id: trafficStep.id,
        }),
      ),
    );

    const seededTasks: SeededTask[] = await Promise.all(
      tasks.map(async (task, index) => {
        const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
          state: "WAITING_FOR_INPUT",
          agentProfileId: seedData.agentProfileId,
          repositoryId: seedData.repositoryId,
        });
        return { id: task.id, title: trafficTaskTitle(index), sessionId };
      }),
    );
    const switchTasks = seededTasks.slice(0, SWITCH_COUNT);
    const switchSessionIds = new Set(switchTasks.map((task) => task.sessionId));

    await testPage.goto(`/t/${switchTasks[0].id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebarTaskItem(switchTasks[0].title)).toHaveAttribute(
      "aria-current",
      "true",
      { timeout: 30_000 },
    );

    await expect
      .poll(() => session.sidebar.getByTestId("sidebar-task-item").count(), {
        timeout: 30_000,
        message: `expected all ${TASK_COUNT} task rows in the sidebar`,
      })
      .toBeGreaterThanOrEqual(TASK_COUNT);

    // Exercise the inactive-session message path while only the selected task
    // is subscribed. Under the old sidebar implementation these writes also
    // produced message detail frames in the browser for every inactive row.
    await Promise.all(
      seededTasks.slice(SWITCH_COUNT).map((task, index) =>
        apiClient.seedSessionMessage(task.sessionId, {
          type: "message",
          content: `inactive traffic probe ${index + 1}`,
        }),
      ),
    );

    for (const task of switchTasks.slice(1)) {
      await session.sidebarTaskItem(task.title).click();
      await expect(session.sidebarTaskItem(task.title)).toHaveAttribute("aria-current", "true", {
        timeout: 15_000,
      });
    }

    // Leave a deliberate five-second observation window after the five task
    // switches. This is a diagnostic total, not a compression-dependent
    // correctness threshold.
    await testPage.waitForTimeout(5_000);

    const summary = summarizeGatewayTraffic(capture.frames);
    await test.info().attach("gateway-traffic.json", {
      body: JSON.stringify(summary, null, 2),
      contentType: "application/json",
    });
    test.info().annotations.push({
      type: "gateway-traffic",
      description: JSON.stringify(summary),
    });
    console.log(`[gateway-traffic] ${JSON.stringify(summary)}`);

    const sentSubscribes = capture.frames.filter(
      (frame) => frame.direction === "sent" && frame.action === "session.subscribe",
    );
    const sentUnsubscribes = capture.frames.filter(
      (frame) => frame.direction === "sent" && frame.action === "session.unsubscribe",
    );

    expect(sentSubscribes.length).toBeLessThanOrEqual(MAX_SUBSCRIPTION_OPERATIONS);
    expect(sentUnsubscribes.length).toBeLessThanOrEqual(MAX_SUBSCRIPTION_OPERATIONS);
    expect(
      sentSubscribes.filter((frame) => !switchSessionIds.has(frame.sessionId ?? "")),
      "inactive task sessions must not be subscribed by the task switcher",
    ).toEqual([]);
    expect(
      sentUnsubscribes.filter((frame) => !switchSessionIds.has(frame.sessionId ?? "")),
      "inactive task sessions must not be unsubscribed by the task switcher",
    ).toEqual([]);

    const inactiveDetailFrames = capture.frames.filter(
      (frame) =>
        frame.direction === "received" &&
        SESSION_DETAIL_ACTIONS.has(frame.action ?? "") &&
        frame.sessionId != null &&
        !switchSessionIds.has(sessionIdFromFrame(frame) ?? ""),
    );
    expect(
      inactiveDetailFrames,
      "inactive task rows must not receive session detail streams",
    ).toEqual([]);
  });
});
