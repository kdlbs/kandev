import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

type GitStatusSnapshot = {
  action?: unknown;
  payload?: {
    type?: unknown;
    session_id?: unknown;
    task_environment_id?: unknown;
    status?: {
      repository_name?: unknown;
      status_state?: unknown;
      files_complete?: unknown;
      detail_state?: unknown;
      files?: Record<string, unknown>;
    };
  };
};

export type GitRefreshTrace = {
  mode: string;
  success: boolean;
  sessionId: string;
  environmentId?: string;
  snapshots: GitStatusSnapshot[];
  forced: boolean;
};

export function createGitEnrichmentGate(tmpDir: string) {
  const delayFile = path.join(tmpDir, "git-delay-ms");
  const startedFile = path.join(tmpDir, "git-status-enrichment-started");
  const releaseFile = path.join(tmpDir, "git-status-enrichment-release");

  return {
    arm() {
      for (const file of [startedFile, releaseFile]) {
        if (fs.existsSync(file)) fs.unlinkSync(file);
      }
      fs.writeFileSync(
        delayFile,
        JSON.stringify({
          subcommand: "diff",
          requiredArgs: ["--numstat"],
          startedFile,
          releaseFile,
        }),
      );
    },
    async waitUntilStarted() {
      await expect
        .poll(() => fs.existsSync(startedFile), {
          timeout: 30_000,
          message: "the real Git enrichment command should reach the controlled gate",
        })
        .toBe(true);
    },
    release() {
      fs.writeFileSync(releaseFile, "release");
    },
    dispose() {
      fs.writeFileSync(releaseFile, "release");
      for (const file of [delayFile, startedFile, releaseFile]) {
        if (fs.existsSync(file)) fs.unlinkSync(file);
      }
    },
  };
}

function parseFrame(value: string): WireFrame | null {
  try {
    const frame = JSON.parse(value) as unknown;
    return typeof frame === "object" && frame !== null ? (frame as WireFrame) : null;
  } catch {
    return null;
  }
}

function recordValue(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function statusEvent(frame: WireFrame): GitStatusSnapshot | null {
  if (frame.type !== "notification" || frame.action !== "session.git.event") return null;
  const payload = recordValue(frame.payload);
  if (payload?.type !== "status_update") return null;
  return frame as GitStatusSnapshot;
}

function snapshotContainsPath(snapshot: GitStatusSnapshot, filePath: string): boolean {
  return Object.prototype.hasOwnProperty.call(snapshot.payload?.status?.files ?? {}, filePath);
}

type BridgeSocket = {
  send(message: string | Buffer): void;
  onMessage(handler: (message: string | Buffer) => void): void;
};

type ClientBridgeSocket = BridgeSocket & { connectToServer(): BridgeSocket };

type GitRefreshBridgeState = {
  requests: string[];
  responses: GitRefreshTrace[];
  pendingEventsDropped: GitStatusSnapshot[];
  readyNotifications: GitStatusSnapshot[];
  forcedFailureModes: Set<string>;
  failureEnvironmentId: string;
  holdFreshGitRefreshRequests: boolean;
  heldFreshGitRefreshRequests: Array<{ frame: string; server: BridgeSocket }>;
  holdCommitDiffRequests: boolean;
  commitDiffRequestCount: number;
  commitDiffResponseCount: number;
  heldCommitDiffRequests: Array<{ frame: string; server: BridgeSocket }>;
};

function holdCommitDiffRequest(
  part: string,
  server: BridgeSocket,
  state: GitRefreshBridgeState,
): boolean {
  state.commitDiffRequestCount += 1;
  if (state.holdCommitDiffRequests) {
    state.heldCommitDiffRequests.push({ frame: part.trim(), server });
    return true;
  }
  return false;
}

function failGitRefreshRequest(
  frame: WireFrame,
  payload: Record<string, unknown> | null,
  mode: string,
  socket: BridgeSocket,
  state: GitRefreshBridgeState,
): boolean {
  if (!state.forcedFailureModes.has(mode)) return false;
  socket.send(
    JSON.stringify({
      id: frame.id,
      type: "response",
      action: frame.action,
      payload: {
        success: false,
        session_id: payload?.session_id,
        task_environment_id: state.failureEnvironmentId,
        mode,
        status_state: "unavailable",
        error_code: "status_timeout",
        snapshots: [],
      },
    }),
  );
  state.responses.push({
    mode,
    success: false,
    sessionId: String(payload?.session_id ?? ""),
    environmentId: state.failureEnvironmentId,
    snapshots: [],
    forced: true,
  });
  return true;
}

function handleGitRefreshRequest(
  part: string,
  socket: BridgeSocket,
  server: BridgeSocket,
  frame: WireFrame,
  state: GitRefreshBridgeState,
): boolean {
  const payload = recordValue(frame.payload);
  const mode = typeof payload?.mode === "string" ? payload.mode : "fresh";
  state.requests.push(mode);
  if (mode === "fresh" && state.holdFreshGitRefreshRequests) {
    state.heldFreshGitRefreshRequests.push({ frame: part.trim(), server });
    return true;
  }
  return failGitRefreshRequest(frame, payload, mode, socket, state);
}

function consumeClientRequest(
  part: string,
  socket: BridgeSocket,
  server: BridgeSocket,
  state: GitRefreshBridgeState,
): boolean {
  const frame = parseFrame(part.trim());
  if (frame?.type !== "request" || typeof frame.id !== "string") return false;
  if (frame.action === "session.commit_diff") {
    return holdCommitDiffRequest(part, server, state);
  }
  if (frame.action !== "session.git.refresh") return false;
  return handleGitRefreshRequest(part, socket, server, frame, state);
}

function recordRefreshResponse(
  frame: WireFrame | null,
  payload: Record<string, unknown> | null,
  state: GitRefreshBridgeState,
) {
  if (frame?.type !== "response" || frame.action !== "session.git.refresh" || !payload) return;
  const snapshots = Array.isArray(payload.snapshots)
    ? (payload.snapshots as GitStatusSnapshot[])
    : [];
  state.responses.push({
    mode: typeof payload.mode === "string" ? payload.mode : "fresh",
    success: payload.success === true,
    sessionId: typeof payload.session_id === "string" ? payload.session_id : "",
    environmentId:
      typeof payload.task_environment_id === "string" ? payload.task_environment_id : undefined,
    snapshots,
    forced: false,
  });
}

function consumeServerFrame(part: string, state: GitRefreshBridgeState): boolean {
  const frame = parseFrame(part.trim());
  const payload = recordValue(frame?.payload);
  if (frame?.type === "response" && frame.action === "session.commit_diff") {
    state.commitDiffResponseCount += 1;
  }
  recordRefreshResponse(frame, payload, state);
  const event = frame ? statusEvent(frame) : null;
  const detailState = event?.payload?.status?.detail_state;
  if (detailState === "pending" && event) {
    state.pendingEventsDropped.push(event);
    return false;
  }
  if (detailState === "ready" && event) state.readyNotifications.push(event);
  return true;
}

function forwardClientMessage(
  message: string | Buffer,
  socket: BridgeSocket,
  server: BridgeSocket,
  state: GitRefreshBridgeState,
) {
  if (typeof message !== "string") {
    server.send(message);
    return;
  }
  const forwarded = message
    .split("\n")
    .filter((part) => !consumeClientRequest(part, socket, server, state));
  const output = forwarded.join("\n");
  if (output.trim()) server.send(output);
}

function forwardServerMessage(
  message: string | Buffer,
  socket: BridgeSocket,
  state: GitRefreshBridgeState,
) {
  if (typeof message !== "string") {
    socket.send(message);
    return;
  }
  const forwarded = message.split("\n").filter((part) => consumeServerFrame(part, state));
  const output = forwarded.join("\n");
  if (output.trim()) socket.send(output);
}

function connectGitRefreshBridge(socket: ClientBridgeSocket, state: GitRefreshBridgeState) {
  const server = socket.connectToServer();
  socket.onMessage((message) => forwardClientMessage(message, socket, server, state));
  server.onMessage((message) => forwardServerMessage(message, socket, state));
}

export async function routeGitStatusRefresh(page: Page) {
  const state: GitRefreshBridgeState = {
    requests: [],
    responses: [],
    pendingEventsDropped: [],
    readyNotifications: [],
    forcedFailureModes: new Set(),
    failureEnvironmentId: "",
    holdFreshGitRefreshRequests: false,
    heldFreshGitRefreshRequests: [],
    holdCommitDiffRequests: false,
    commitDiffRequestCount: 0,
    commitDiffResponseCount: 0,
    heldCommitDiffRequests: [],
  };

  await page.routeWebSocket(/\/ws$/, (socket) => connectGitRefreshBridge(socket, state));

  return {
    setFailureEnvironmentId(environmentId: string) {
      state.failureEnvironmentId = environmentId;
    },
    forceFailures(modes: string[]) {
      state.forcedFailureModes = new Set(modes);
    },
    allowResponses() {
      state.forcedFailureModes = new Set();
    },
    holdFreshGitRefreshRequests() {
      state.holdFreshGitRefreshRequests = true;
    },
    async waitForHeldFreshGitRefreshRequests(count: number) {
      await expect
        .poll(() => state.heldFreshGitRefreshRequests.length, {
          timeout: 30_000,
          message: "the expected fresh Git refresh request should be held",
        })
        .toBeGreaterThanOrEqual(count);
    },
    releaseFreshGitRefreshRequests() {
      state.holdFreshGitRefreshRequests = false;
      for (const request of state.heldFreshGitRefreshRequests.splice(0)) {
        request.server.send(request.frame);
      }
    },
    holdCommitDiffRequests() {
      state.holdCommitDiffRequests = true;
    },
    async waitForCommitDiffRequests(count: number) {
      await expect
        .poll(() => state.commitDiffRequestCount, {
          timeout: 30_000,
          message: "the expected inline commit diff requests should arrive",
        })
        .toBeGreaterThanOrEqual(count);
    },
    async waitForCommitDiffResponses(count: number) {
      await expect
        .poll(() => state.commitDiffResponseCount, {
          timeout: 30_000,
          message: "the released inline commit diff request should complete",
        })
        .toBeGreaterThanOrEqual(count);
    },
    releaseNextCommitDiffRequest() {
      const request = state.heldCommitDiffRequests.shift();
      if (!request) throw new Error("No held inline commit diff request is available to release");
      request.server.send(request.frame);
    },
    releaseCommitDiffRequests() {
      state.holdCommitDiffRequests = false;
      for (const request of state.heldCommitDiffRequests.splice(0)) {
        request.server.send(request.frame);
      }
    },
    requestCount(mode: string) {
      return state.requests.filter((candidate) => candidate === mode).length;
    },
    responseCount(mode: string) {
      return state.responses.filter((candidate) => candidate.mode === mode).length;
    },
    droppedPendingCount() {
      return state.pendingEventsDropped.length;
    },
    readyNotificationCount() {
      return state.readyNotifications.length;
    },
    async waitForResponse(mode: string, afterCount = 0) {
      await expect
        .poll(() => state.responses.filter((candidate) => candidate.mode === mode).length, {
          timeout: 30_000,
          message: `the correlated ${mode} Git refresh response should arrive`,
        })
        .toBeGreaterThan(afterCount);
      return state.responses.filter((candidate) => candidate.mode === mode).at(-1)!;
    },
    async waitForDroppedPendingStatus() {
      await expect
        .poll(() => state.pendingEventsDropped.length, {
          timeout: 30_000,
          message: "the pending Git status notification should be deliberately dropped",
        })
        .toBeGreaterThan(0);
    },
    async waitForReadyNotification() {
      await expect
        .poll(() => state.readyNotifications.length, {
          timeout: 30_000,
          message: "the tracker should publish its enriched status after the gate opens",
        })
        .toBeGreaterThan(0);
    },
    responseIncludesFile(mode: string, filePath: string) {
      return state.responses.some(
        (response) =>
          response.mode === mode &&
          response.snapshots.some((snapshot) => snapshotContainsPath(snapshot, filePath)),
      );
    },
    responseHasPendingDetails(mode: string, filePath: string) {
      return state.responses.some(
        (response) =>
          response.mode === mode &&
          response.snapshots.some(
            (snapshot) =>
              snapshotContainsPath(snapshot, filePath) &&
              snapshot.payload?.status?.detail_state === "pending",
          ),
      );
    },
    notificationHasReadyFile(filePath: string) {
      return state.readyNotifications.some((event) => snapshotContainsPath(event, filePath));
    },
  };
}
