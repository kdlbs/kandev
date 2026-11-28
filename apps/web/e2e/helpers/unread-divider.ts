import { randomUUID } from "node:crypto";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { withDatabase } from "./interrupted-prompt-recovery";
import { multiMessageScript } from "./seed-session-messages";
import { createFailOnResumeProfile } from "./session-resume-prompt-queue";
import { waitForSessionDone } from "./session";
import type { WsFrame, WsWatcher } from "./causal-waits";
import { attachGatewayTrafficCapture } from "./ws-traffic";

/** True only when element is actually scrolled into container's visible
 *  viewport. Unlike Playwright's toBeVisible(), which only checks CSS
 *  visibility, this also verifies the element is scrolled into the
 *  container's viewport, not merely present somewhere in its (taller)
 *  overflow area. Callers must root both locators at
 *  SessionPage.activeChat() — Dockview keeps background chat panels
 *  mounted, and an unscoped locator could otherwise resolve a stale,
 *  hidden panel's elements instead of the one the user is actually
 *  looking at. */
export async function isScrolledIntoView(container: Locator, element: Locator): Promise<boolean> {
  const [containerBox, elementBox] = await Promise.all([
    container.boundingBox(),
    element.boundingBox(),
  ]);
  if (!containerBox || !elementBox) return false;
  return (
    elementBox.y >= containerBox.y &&
    elementBox.y + elementBox.height <= containerBox.y + containerBox.height
  );
}

/**
 * Seeds a task whose transcript buries a read cursor under enough messages
 * both before and after it that, scrolled to the bottom (the naive,
 * unconditional behavior this feature replaces), the divider would land
 * well outside the visible viewport. Shared by the desktop and mobile
 * scroll-to-divider specs — same setup, different viewport/project.
 */
export async function seedScrollTestConversation(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{
  taskId: string;
  sessionId: string;
  cursorMessageId: string;
  newestMessageId: string;
}> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: multiMessageScript(["chat ready"], 5),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  const sessionId = task.session_id;

  await waitForSessionDone(apiClient, task.id, sessionId, "initial unread-divider conversation");

  await apiClient.seedAgentMessages(sessionId, 40, "before cursor");
  const beforeCursor = await apiClient.listSessionMessages(sessionId);
  const cursorMessageId = beforeCursor.messages[beforeCursor.messages.length - 1].id;
  await apiClient.seedAgentMessages(sessionId, 40, "after cursor");
  const fullTranscript = await apiClient.listSessionMessages(sessionId);
  const newestMessageId = fullTranscript.messages[fullTranscript.messages.length - 1].id;

  await apiClient.forceSetSessionReadCursor(sessionId, cursorMessageId);

  return { taskId: task.id, sessionId, cursorMessageId, newestMessageId };
}

export const TASK_UNREAD_DOT_LABEL = "Task has unread messages";

/** Arms a wait for the task status summary that lists `taskId` as unread.
 *  Arm it before the action that makes the backend publish the summary. */
export function waitForTaskUnreadSummary(ws: WsWatcher, taskId: string): Promise<WsFrame> {
  return ws.waitForEvent("task.status_summary.updated", {
    where: (payload) => {
      const summary = payload.status_summary as { unread_session_ids?: string[] } | undefined;
      return payload.task_id === taskId && (summary?.unread_session_ids?.length ?? 0) > 0;
    },
  });
}

export type UnreadDotTasks = {
  navTitle: string;
  targetTitle: string;
  cursorlessTitle: string;
  navTaskId: string;
  navSessionId: string;
  targetTaskId: string;
  targetSessionId: string;
  cursorlessSessionId: string;
};

/**
 * Seeds three agent-less tasks for the task-row unread dot: a navigation task
 * the spec opens, a target whose session has a read cursor at its tail, and a
 * task whose session was never read (no cursor, so its agent output is
 * unread). The cursor is force-set through the e2e route, which also moves the
 * live task-status projection.
 */
export async function seedUnreadDotTasks(
  apiClient: ApiClient,
  seedData: SeedData,
  prefix: string,
): Promise<UnreadDotTasks> {
  const stepOptions = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const titles = {
    navTitle: `${prefix} navigation`,
    targetTitle: `${prefix} target`,
    cursorlessTitle: `${prefix} never opened`,
  };
  const sessionFor = async (title: string) => {
    const task = await apiClient.seedTask(seedData.workspaceId, title, stepOptions);
    const session = await apiClient.seedTaskSession(task.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    return { taskId: task.task_id, sessionId: session.session_id };
  };
  const nav = await sessionFor(titles.navTitle);
  const target = await sessionFor(titles.targetTitle);
  const cursorless = await sessionFor(titles.cursorlessTitle);

  const read = await apiClient.seedSessionMessage(target.sessionId, {
    type: "message",
    content: "already read",
  });
  await apiClient.forceSetSessionReadCursor(target.sessionId, read.messageId);
  await apiClient.seedSessionMessage(cursorless.sessionId, {
    type: "message",
    content: "never read",
  });

  return {
    ...titles,
    navTaskId: nav.taskId,
    navSessionId: nav.sessionId,
    targetTaskId: target.taskId,
    targetSessionId: target.sessionId,
    cursorlessSessionId: cursorless.sessionId,
  };
}

/** Row height in CSS pixels; the dot must never change it. */
export async function rowHeight(row: Locator): Promise<number> {
  const box = await row.boundingBox();
  if (!box) throw new Error("expected a measurable task row");
  return box.height;
}

export type UnreadSettings = {
  unread_divider?: boolean;
  unread_task_dot?: boolean;
  unread_task_dot_hide_delay_seconds?: number;
  agent_tab_close_behavior?: "hide_panel" | "delete_session";
};

/** The unread settings a spec changes, captured so it can restore them. */
export async function captureUnreadSettings(apiClient: ApiClient): Promise<UnreadSettings> {
  const { settings } = await apiClient.getUserSettings();
  return {
    unread_divider: settings.unread_divider as boolean | undefined,
    unread_task_dot: settings.unread_task_dot as boolean | undefined,
    unread_task_dot_hide_delay_seconds: settings.unread_task_dot_hide_delay_seconds as
      | number
      | undefined,
    agent_tab_close_behavior: settings.agent_tab_close_behavior as
      | "hide_panel"
      | "delete_session"
      | undefined,
  };
}

export const FAILED_SESSION_ERROR = "Agent produced no output since start.";

export type FailedSiblingTask = {
  navTitle: string;
  navTaskId: string;
  title: string;
  taskId: string;
  primarySessionId: string;
  primaryCursor: string;
  failedSessionId: string;
  failedTailMessageId: string;
  liveSessionId: string;
  liveTailMessageId: string;
};

function minutesAgo(minutes: number): string {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

/**
 * Seeds the reported mixed-session shape: a read primary waiting for input, a
 * cursorless FAILED sibling whose stored boot/status/error output has no
 * execution behind it, and an unread RUNNING sibling. A separate task gives the
 * spec somewhere to navigate to. Sessions use explicit IDs because the harness
 * otherwise reuses one row per task and agent profile.
 */
export async function seedFailedSiblingTask(
  apiClient: ApiClient,
  seedData: SeedData,
  prefix: string,
): Promise<FailedSiblingTask> {
  const stepOptions = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const navTitle = `${prefix} navigation`;
  const title = `${prefix} mixed sessions`;
  const nav = await apiClient.seedTask(seedData.workspaceId, navTitle, stepOptions);
  await apiClient.seedTaskSession(nav.task_id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  const task = await apiClient.seedTask(seedData.workspaceId, title, stepOptions);
  const seedSession = (
    state: "FAILED" | "RUNNING" | "WAITING_FOR_INPUT",
    startedMinutesAgo: number,
    extra: { completedAt?: string; errorMessage?: string } = {},
  ) =>
    apiClient.seedTaskSession(task.task_id, {
      state,
      sessionId: randomUUID(),
      agentProfileId: seedData.agentProfileId,
      startedAt: minutesAgo(startedMinutesAgo),
      ...extra,
    });
  const failed = await seedSession("FAILED", 30, {
    completedAt: minutesAgo(25),
    errorMessage: "agent produced no output since start",
  });
  const live = await seedSession("RUNNING", 20);
  const primary = await seedSession("WAITING_FOR_INPUT", 10);
  await apiClient.setPrimarySession(primary.session_id);

  await apiClient.seedSessionMessage(failed.session_id, {
    type: "message",
    content: "Start the failed sibling",
    authorType: "user",
  });
  await apiClient.seedSessionMessage(failed.session_id, {
    type: "script_execution",
    content: "",
    metadata: {
      agent_name: "mock",
      command: "mock-agent acp",
      exit_code: 0,
      is_resuming: false,
      script_type: "agent_boot",
      status: "exited",
    },
  });
  await apiClient.seedSessionMessage(failed.session_id, {
    type: "status",
    content: "New session started",
  });
  const failedTail = await apiClient.seedSessionMessage(failed.session_id, {
    type: "error",
    content: FAILED_SESSION_ERROR,
    metadata: { session_id: failed.session_id, task_id: task.task_id },
  });
  const liveTail = await apiClient.seedSessionMessage(live.session_id, {
    type: "message",
    content: "live sibling output",
  });
  const primaryRead = await apiClient.seedSessionMessage(primary.session_id, {
    type: "message",
    content: "primary output already read",
  });
  await apiClient.forceSetSessionReadCursor(primary.session_id, primaryRead.messageId);

  return {
    navTitle,
    navTaskId: nav.task_id,
    title,
    taskId: task.task_id,
    primarySessionId: primary.session_id,
    primaryCursor: primaryRead.messageId,
    failedSessionId: failed.session_id,
    failedTailMessageId: failedTail.messageId,
    liveSessionId: live.session_id,
    liveTailMessageId: liveTail.messageId,
  };
}

export type FailingResumeTask = {
  navTitle: string;
  navTaskId: string;
  title: string;
  taskId: string;
  failingSessionId: string;
  failingCursor: string;
  siblingSessionId: string;
  siblingCursor: string;
  profileId: string;
};

/**
 * Seeds the reported failed-resume shape with a real agent: a primary session
 * with more than one history page, read through its newest row, whose saved
 * agent refuses to resume. Opening the task resumes it automatically; the
 * resume fails and stores a resume boot row plus a recovery card after the
 * cursor. A read sibling waiting for input shares its environment.
 */
export async function seedFailingResumeTask(
  apiClient: ApiClient,
  seedData: SeedData,
  backendTmpDir: string,
  prefix: string,
): Promise<FailingResumeTask> {
  const stepOptions = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const navTitle = `${prefix} navigation`;
  const nav = await apiClient.seedTask(seedData.workspaceId, navTitle, stepOptions);
  await apiClient.seedTaskSession(nav.task_id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  const profile = await createFailOnResumeProfile(apiClient, `${prefix} ${randomUUID()}`);
  const title = `${prefix} failed resume`;
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description: "/e2e:simple-message",
    repository_ids: [seedData.repositoryId],
    ...stepOptions,
  });
  const failingSessionId = task.session_id;
  if (!failingSessionId) throw new Error("failed-resume task has no session");
  await waitForSessionDone(apiClient, task.id, failingSessionId, "initial prompt", 60_000);
  for (let index = 0; index < 105; index++) {
    await apiClient.seedSessionMessage(failingSessionId, {
      type: index % 3 === 0 ? "tool_read" : "thinking",
      content: `stored history ${index}`,
    });
  }
  const failingCursor = await newestStoredMessageId(apiClient, failingSessionId);
  await apiClient.forceSetSessionReadCursor(failingSessionId, failingCursor);
  await apiClient.stopSession({ session_id: failingSessionId, force: true });
  await expect
    .poll(
      async () =>
        (
          await apiClient.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
            task_id: task.id,
            session_id: failingSessionId,
          })
        ).is_agent_running,
      { timeout: 30_000 },
    )
    .toBe(false);

  const sibling = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
  });
  const siblingRead = await apiClient.seedSessionMessage(sibling.session_id, {
    type: "message",
    content: "sibling output already read",
  });
  await apiClient.forceSetSessionReadCursor(sibling.session_id, siblingRead.messageId);
  // Make the stopped session resumable on the next visit, as after a restart.
  withDatabase(backendTmpDir, (db) => {
    db.prepare(
      `UPDATE task_sessions SET state = 'WAITING_FOR_INPUT', error_message = '',
       metadata = json_remove(metadata, '$.last_agent_error', '$.agent_delivery_recovery')
       WHERE id = ?`,
    ).run(failingSessionId);
    db.prepare(
      `UPDATE task_sessions SET task_environment_id =
       (SELECT task_environment_id FROM task_sessions WHERE id = ?) WHERE id = ?`,
    ).run(failingSessionId, sibling.session_id);
  });
  return {
    navTitle,
    navTaskId: nav.task_id,
    title,
    taskId: task.id,
    failingSessionId,
    failingCursor,
    siblingSessionId: sibling.session_id,
    siblingCursor: siblingRead.messageId,
    profileId: profile.id,
  };
}

/** Stops the seeded agent and removes its failing profile. */
export async function cleanupFailingResumeTask(
  apiClient: ApiClient,
  seeded: FailingResumeTask,
): Promise<void> {
  await apiClient
    .stopSession({ session_id: seeded.failingSessionId, force: true })
    .catch(() => undefined);
  await apiClient.deleteAgentProfile(seeded.profileId, true).catch(() => undefined);
}

/** The newest stored message of a session; reading it to its end acknowledges this row. */
export async function newestStoredMessageId(
  apiClient: ApiClient,
  sessionId: string,
): Promise<string> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  const newest = messages.at(-1);
  if (!newest) throw new Error(`Session ${sessionId} has no stored messages`);
  return newest.id;
}

/** Readback must contain the new failed resume boot followed by its recovery card. */
export async function failedResumeTail(
  apiClient: ApiClient,
  seeded: FailingResumeTask,
): Promise<string> {
  const { messages } = await apiClient.listSessionMessages(seeded.failingSessionId);
  const cursorIndex = messages.findIndex((message) => message.id === seeded.failingCursor);
  expect(cursorIndex).toBeGreaterThanOrEqual(0);
  const added = messages.slice(cursorIndex + 1);
  const bootIndex = added.findIndex(
    (message) =>
      message.type === "script_execution" &&
      message.metadata?.script_type === "agent_boot" &&
      message.metadata?.is_resuming === true &&
      message.metadata?.status === "failed",
  );
  expect(bootIndex).toBeGreaterThanOrEqual(0);
  const card = added.at(-1);
  expect(added.length - 1).toBeGreaterThan(bootIndex);
  expect(card?.type).toBe("status");
  expect(card?.metadata?.recovery_actions).toBe(true);
  return card!.id;
}

/** Arm both independent notification streams before opening the task. */
export function waitForFailedResume(
  watcher: WsWatcher,
  seeded: FailingResumeTask,
): Promise<WsFrame[]> {
  return Promise.all([
    watcher.waitForEvent("session.state_changed", {
      timeout: 60_000,
      where: (payload) =>
        payload.task_id === seeded.taskId &&
        payload.session_id === seeded.failingSessionId &&
        payload.new_state === "FAILED",
    }),
    watcher.waitForEvent("task.updated", {
      timeout: 60_000,
      where: (payload) =>
        payload.task_id === seeded.taskId && payload.primary_session_id === seeded.siblingSessionId,
    }),
  ]);
}

/** The authoritative (backend-projected) unread session IDs for a task, sorted. */
export async function taskUnreadSessionIds(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<string[]> {
  const { tasks } = await apiClient.listTasks(workspaceId);
  const matchingTasks = tasks.filter((task) => task.id === taskId);
  if (matchingTasks.length !== 1)
    throw new Error(`Expected exactly one authoritative task ${taskId}`);
  const summary = matchingTasks[0].status_summary;
  if (summary == null) throw new Error(`Missing authoritative summary for task ${taskId}`);
  return [...(summary.unread_session_ids ?? [])].sort();
}

/** The persisted read cursor and lifecycle of one session. */
export async function sessionReadState(
  apiClient: ApiClient,
  sessionId: string,
): Promise<{ cursor: string | null; state: string; executionId: string | null }> {
  const { session } = await apiClient.getTaskSession(sessionId);
  return {
    cursor: session.last_read_message_id ?? null,
    state: session.state,
    executionId: session.agent_execution_id ?? null,
  };
}

const AGENT_START_ACTIONS: readonly string[] = [
  "session.launch",
  "session.ensure",
  "session.recover",
  "task.launch.recover",
];

export type ReadTrafficObserver = {
  /** Session IDs and message IDs of every mark-read request the page sent. */
  markReads: Array<{ sessionId: string; messageId: string }>;
  /** Agent start/resume/recover requests the page sent over the gateway. */
  agentStarts: () => string[];
  /** Gateway frames the page sent, proving the observer is attached. */
  sentFrameCount: () => number;
};

/** Records mark-read and agent-start traffic. Arm before navigating. */
export function observeReadTraffic(page: Page): ReadTrafficObserver {
  const markReads: ReadTrafficObserver["markReads"] = [];
  page.on("request", (request) => {
    const match = /\/api\/v1\/task-sessions\/([^/]+)\/mark-read$/.exec(
      new URL(request.url()).pathname,
    );
    if (!match || request.method() !== "POST") return;
    const body = request.postDataJSON() as { message_id: string };
    markReads.push({ sessionId: match[1], messageId: body.message_id });
  });
  const capture = attachGatewayTrafficCapture(page);
  const sent = () => capture.frames.filter((frame) => frame.direction === "sent");
  return {
    markReads,
    agentStarts: () =>
      sent()
        .filter((frame) => frame.action !== undefined && AGENT_START_ACTIONS.includes(frame.action))
        .map((frame) => `${frame.action}:${frame.sessionId ?? ""}`),
    sentFrameCount: () => sent().length,
  };
}
