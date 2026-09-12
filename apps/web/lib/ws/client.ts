/* eslint-disable max-lines -- WebSocketClient intentionally owns one connection's complete request, subscription, and reconnect lifecycle. */

import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { ConnectionStatus } from "@/lib/types/connection";
import { generateUUID } from "@/lib/utils";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { dispatchToPluginWsHandlers } from "@/lib/ws/plugin-bridge";
import { toWebSocketRequestError } from "./request-error";
import {
  isRawSessionEvent,
  orderedCoreDisposition,
  orderedCoreAction,
  projectCoreSessionPayload,
  type CoreSessionStream,
  type RawSessionEvent,
} from "./ordered-session-events";
export type { RawSessionEvent } from "./ordered-session-events";
export { WebSocketRequestError, type WebSocketRequestErrorDetails } from "./request-error";

const debugDispatch = createDebugLogger("ws:dispatch");

// High-frequency notification types we skip in the dispatch log to avoid
// drowning the console during agent streams. Filter [ws:dispatch] to see
// everything else; if you need the streaming traffic, comment this out.
const DISPATCH_LOG_DENYLIST = new Set<string>([
  "session.message.added",
  "session.message.updated",
  "session.message.deleted",
  "session.shell.output",
  "session.process.output",
]);

type MessageHandler<T extends BackendMessageType> = (message: BackendMessageMap[T]) => void;

// Internal alias for the status vocabulary shared with the UI.
type WebSocketStatus = ConnectionStatus;

export interface ReconnectOptions {
  enabled?: boolean;
  maxAttempts?: number;
  initialDelay?: number;
  maxDelay?: number;
  backoffMultiplier?: number;
}

export interface SessionSubscriptionHandle {
  ready: Promise<void>;
  unsubscribe: () => void;
}

type SessionSubscriptionReadiness = {
  promise: Promise<void>;
  resolve: () => void;
  reject: (reason: unknown) => void;
  requestStarted: boolean;
  settled: boolean;
};

type OrderedSessionResponse = Record<string, unknown>;

// i18n-exempt: transport protocol validation diagnostics are never rendered as user-facing copy.
function invalidOrderedSessionResponse(): Error {
  return new Error("Invalid ordered session response");
}

function isNonNegativeSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function requiredOrderedString(response: OrderedSessionResponse, key: string): string {
  const value = response[key];
  if (typeof value !== "string" || value === "") throw invalidOrderedSessionResponse();
  return value;
}

// eslint-disable-next-line complexity -- The wire contract is validated field-by-field before state mutation.
function validateOrderedSubscribeResponse(
  response: unknown,
  sessionId: string,
  wireId: string,
): { eventWatermark: number; resumeToken: string; result: "fresh" | "replay" | "invalid_resume" } {
  if (!response || typeof response !== "object" || Array.isArray(response)) {
    throw invalidOrderedSessionResponse();
  }
  const payload = response as OrderedSessionResponse;
  if (payload.success !== true || payload.session_id !== sessionId || payload.wire_id !== wireId) {
    throw invalidOrderedSessionResponse();
  }
  const eventWatermark = payload.event_watermark;
  if (
    !isNonNegativeSafeInteger(eventWatermark) ||
    payload.snapshot_cutoff !== eventWatermark ||
    !["fresh", "replay", "invalid_resume"].includes(payload.result as string)
  ) {
    throw invalidOrderedSessionResponse();
  }
  requiredOrderedString(payload, "snapshot_token");
  const resumeToken = requiredOrderedString(payload, "resume_token");
  requiredOrderedString(payload, "expires_at");
  if (payload.result === "replay") {
    const replayFrom = payload.replay_from;
    const replayTo = payload.replay_to;
    if (
      !isNonNegativeSafeInteger(replayFrom) ||
      replayFrom === 0 ||
      !isNonNegativeSafeInteger(replayTo) ||
      replayTo < replayFrom ||
      replayTo > eventWatermark
    ) {
      throw invalidOrderedSessionResponse();
    }
  }
  return {
    eventWatermark,
    resumeToken,
    result: payload.result as "fresh" | "replay" | "invalid_resume",
  };
}

function validateOrderedAckResponse(
  response: unknown,
  sessionId: string,
  wireId: string,
  sequence: number,
): string {
  if (!response || typeof response !== "object" || Array.isArray(response)) {
    throw invalidOrderedSessionResponse();
  }
  const payload = response as OrderedSessionResponse;
  if (
    payload.success !== true ||
    payload.session_id !== sessionId ||
    payload.wire_id !== wireId ||
    payload.acknowledged_sequence !== sequence
  ) {
    throw invalidOrderedSessionResponse();
  }
  return requiredOrderedString(payload, "resume_token");
}
type RawWebSocketMessage = {
  type?: unknown;
  id?: unknown;
  payload?: unknown;
};

const DEFAULT_RECONNECT_OPTIONS: Required<ReconnectOptions> = {
  enabled: true,
  maxAttempts: 10,
  initialDelay: 1000,
  maxDelay: 30000,
  backoffMultiplier: 1.5,
};
// i18n-exempt: transport/API diagnostic. Callers branch on the error code and
// render translated copy; this text only ever appears in a console or as an
// interpolated English diagnostic (see docs/i18n.md on interpolated values).
const WEBSOCKET_CONNECTION_CLOSED_ERROR = "WebSocket connection closed";

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private status: WebSocketStatus = "disconnected";
  private handlers = new Map<BackendMessageType, Set<MessageHandler<BackendMessageType>>>();
  private pendingRequests = new Map<
    string,
    {
      resolve: (payload: unknown) => void;
      reject: (error: Error) => void;
      timeout: ReturnType<typeof setTimeout>;
    }
  >();
  private rawSessionEventHandlers = new Set<(event: RawSessionEvent) => void>();
  private statusHandlers = new Set<(status: WebSocketStatus) => void>();
  private pendingQueue: string[] = [];
  private reconnectOptions: Required<ReconnectOptions>;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalClose = false;
  private subscriptions = new Map<string, number>();
  private sessionSubscriptions = new Map<string, number>();
  private sessionSubscriptionReadiness = new Map<string, SessionSubscriptionReadiness>();
  private coreSessionStreams = new Map<string, CoreSessionStream>();
  // Ref-counted focus signals: a session can be focused by both the task panel
  // and the task details page if both are mounted. Backend wakes its workspace
  // tracker into fast-poll mode while any client has focus, falling back to
  // slow when the count reaches 0 (debounced server-side).
  private sessionFocusCounts = new Map<string, number>();
  private userSubscriptionCount = 0;
  private runSubscriptions = new Map<string, number>();
  private systemMetricsSubscriptionCount = 0;

  constructor(
    private url: string,
    private onStatusChange?: (status: WebSocketStatus) => void,
    reconnectOptions?: ReconnectOptions,
  ) {
    this.reconnectOptions = { ...DEFAULT_RECONNECT_OPTIONS, ...reconnectOptions };
  }

  getStatus() {
    return this.status;
  }

  connect() {
    if (this.socket) return;
    this.intentionalClose = false;
    this.clearReconnectTimer();
    this.setStatus("connecting");
    const socket = new WebSocket(this.url);
    this.socket = socket;

    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.reconnectAttempts = 0;
      this.setStatus("connected");
      this.resubscribe();
      this.flushQueue();
    };

    socket.onmessage = (event) => {
      if (this.socket !== socket) return;
      const parts = (event.data as string).split("\n");
      for (const part of parts) {
        const trimmed = part.trim();
        if (!trimmed) continue;
        try {
          const message = JSON.parse(trimmed) as BackendMessageMap[BackendMessageType];
          this.handleParsedMessage(message);
        } catch {
          // Ignore parse errors for individual messages
        }
      }
    };

    socket.onerror = () => (this.socket === socket ? this.setStatus("error") : undefined);

    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.handleDisconnect(event);
    };
  }

  disconnect() {
    this.intentionalClose = true;
    this.clearReconnectTimer();
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.setStatus("disconnected");
    this.resetSessionSubscriptionReadiness(new Error(WEBSOCKET_CONNECTION_CLOSED_ERROR));
    this.cleanupPendingRequests();
  }

  send(payload: unknown) {
    const data = JSON.stringify(payload);
    if (isDebug()) {
      const p = payload as { action?: string; id?: string; type?: string } | null;
      const action = p?.action ?? "?";
      if (!DISPATCH_LOG_DENYLIST.has(action)) {
        const sessionId = (p as { payload?: { session_id?: string } } | null)?.payload?.session_id;
        debugDispatch("send", {
          action,
          id: p?.id ?? null,
          type: p?.type ?? null,
          sessionId: sessionId ?? null,
          queued: this.status !== "connected" || !this.socket,
        });
      }
    }
    if (this.status !== "connected" || !this.socket) {
      this.pendingQueue.push(data);
      return;
    }
    this.socket.send(data);
  }

  request<T>(action: string, payload: unknown, timeoutMs = 5000): Promise<T> {
    const id = generateUUID();
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error(`WebSocket request timed out: ${action}`));
      }, timeoutMs);
      this.pendingRequests.set(id, {
        resolve: resolve as (payload: unknown) => void,
        reject,
        timeout,
      });
      this.send({ id, type: "request", action, payload });
    });
  }

  subscribe(taskId: string) {
    const currentCount = this.subscriptions.get(taskId) ?? 0;
    const nextCount = currentCount + 1;
    this.subscriptions.set(taskId, nextCount);
    if (this.status === "connected" && nextCount === 1) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "task.subscribe",
        payload: { task_id: taskId },
      });
    }
    return () => this.unsubscribe(taskId);
  }

  subscribeSession(sessionId: string) {
    return this.subscribeSessionWithReady(sessionId).unsubscribe;
  }

  subscribeSessionWithReady(sessionId: string): SessionSubscriptionHandle {
    const currentCount = this.sessionSubscriptions.get(sessionId) ?? 0;
    const nextCount = currentCount + 1;
    this.sessionSubscriptions.set(sessionId, nextCount);
    const readiness = this.getOrCreateSessionSubscriptionReadiness(sessionId);
    if (this.status === "connected" && this.socket) {
      this.startSessionSubscription(sessionId, readiness);
    }
    return {
      ready: readiness.promise,
      unsubscribe: () => this.unsubscribeSession(sessionId),
    };
  }

  resubscribeSession(sessionId: string): Promise<void> {
    if (!this.sessionSubscriptions.has(sessionId)) return Promise.resolve();
    const readiness = this.getOrCreateSessionSubscriptionReadiness(sessionId, true);
    if (this.status === "connected" && this.socket) {
      this.startSessionSubscription(sessionId, readiness);
    }
    return readiness.promise;
  }

  getSessionSubscriptionReadiness(sessionId: string): Promise<void> {
    if (!this.sessionSubscriptions.has(sessionId)) return Promise.resolve();
    if (
      !this.sessionSubscriptionReadiness.has(sessionId) &&
      (this.status === "disconnected" || this.status === "error")
    ) {
      const unavailable = Promise.reject<void>(new Error(WEBSOCKET_CONNECTION_CLOSED_ERROR));
      void unavailable.catch(() => undefined);
      return unavailable;
    }
    const readiness = this.getOrCreateSessionSubscriptionReadiness(sessionId);
    if (this.status === "connected" && this.socket) {
      this.startSessionSubscription(sessionId, readiness);
    }
    return readiness.promise;
  }

  focusSession(sessionId: string) {
    const currentCount = this.sessionFocusCounts.get(sessionId) ?? 0;
    const nextCount = currentCount + 1;
    this.sessionFocusCounts.set(sessionId, nextCount);
    if (this.status === "connected" && nextCount === 1) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "session.focus",
        payload: { session_id: sessionId },
      });
    }
    return () => this.unfocusSession(sessionId);
  }

  /**
   * Request a fresh git snapshot for an already-focused session without
   * changing its focus ref-count. Focus itself is intentionally ACK-only;
   * explicit git refresh keeps tab activation from replaying session data during
   * ordinary task switching.
   */
  refreshSessionData(sessionId: string) {
    if (this.status !== "connected") return;
    if (!this.sessionFocusCounts.get(sessionId)) return;
    this.send({
      id: generateUUID(),
      type: "request",
      action: "session.git.refresh",
      payload: { session_id: sessionId },
    });
  }

  unfocusSession(sessionId: string) {
    const currentCount = this.sessionFocusCounts.get(sessionId);
    if (!currentCount) return;
    const nextCount = currentCount - 1;
    if (nextCount <= 0) {
      this.sessionFocusCounts.delete(sessionId);
      if (this.status === "connected") {
        this.send({
          id: generateUUID(),
          type: "request",
          action: "session.unfocus",
          payload: { session_id: sessionId },
        });
      }
      return;
    }
    this.sessionFocusCounts.set(sessionId, nextCount);
  }

  subscribeUser() {
    this.userSubscriptionCount += 1;
    if (this.status === "connected" && this.userSubscriptionCount === 1) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "user.subscribe",
        payload: {},
      });
    }
  }

  unsubscribe(taskId: string) {
    const currentCount = this.subscriptions.get(taskId);
    if (!currentCount) return;
    const nextCount = currentCount - 1;
    if (nextCount <= 0) {
      this.subscriptions.delete(taskId);
      if (this.status === "connected") {
        this.send({
          id: generateUUID(),
          type: "request",
          action: "task.unsubscribe",
          payload: { task_id: taskId },
        });
      }
      return;
    }
    this.subscriptions.set(taskId, nextCount);
  }

  unsubscribeSession(sessionId: string) {
    const currentCount = this.sessionSubscriptions.get(sessionId);
    if (!currentCount) return;
    const nextCount = currentCount - 1;

    if (nextCount <= 0) {
      this.sessionSubscriptions.delete(sessionId);
      this.cancelSessionSubscriptionReadiness(sessionId);
      const stream = this.coreSessionStreams.get(sessionId);
      if (this.status === "connected" && stream) {
        this.send({
          id: generateUUID(),
          type: "request",
          action: "session.unsubscribe",
          payload: {
            session_id: sessionId,
            consumer_kind: "core",
            wire_id: stream.wireId,
          },
        });
      }
      this.coreSessionStreams.delete(sessionId);
      if (this.status === "connected") {
        this.send({
          id: generateUUID(),
          type: "request",
          action: "session.unsubscribe",
          payload: { session_id: sessionId },
        });
      }
      return;
    }
    this.sessionSubscriptions.set(sessionId, nextCount);
  }

  /**
   * Subscribe to office run-event notifications for the given run id.
   * Ref-counted so multiple components can subscribe independently;
   * only the first call sends `run.subscribe` over the wire, only the
   * last unsubscribe sends `run.unsubscribe`. Returns an
   * unsubscribe function.
   */
  subscribeRun(runId: string) {
    const currentCount = this.runSubscriptions.get(runId) ?? 0;
    const nextCount = currentCount + 1;
    this.runSubscriptions.set(runId, nextCount);
    if (this.status === "connected" && nextCount === 1) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "run.subscribe",
        payload: { run_id: runId },
      });
    }
    return () => this.unsubscribeRun(runId);
  }

  subscribeSystemMetrics() {
    this.systemMetricsSubscriptionCount += 1;
    if (this.status === "connected" && this.systemMetricsSubscriptionCount === 1) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "system.metrics.subscribe",
        payload: {},
      });
    }
    return () => this.unsubscribeSystemMetrics();
  }

  unsubscribeSystemMetrics() {
    this.systemMetricsSubscriptionCount = Math.max(0, this.systemMetricsSubscriptionCount - 1);
    if (this.status === "connected" && this.systemMetricsSubscriptionCount === 0) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "system.metrics.unsubscribe",
        payload: {},
      });
    }
  }

  unsubscribeRun(runId: string) {
    const currentCount = this.runSubscriptions.get(runId);
    if (!currentCount) return;
    const nextCount = currentCount - 1;
    if (nextCount <= 0) {
      this.runSubscriptions.delete(runId);
      if (this.status === "connected") {
        this.send({
          id: generateUUID(),
          type: "request",
          action: "run.unsubscribe",
          payload: { run_id: runId },
        });
      }
      return;
    }
    this.runSubscriptions.set(runId, nextCount);
  }

  unsubscribeUser() {
    this.userSubscriptionCount = Math.max(0, this.userSubscriptionCount - 1);
    if (this.status === "connected" && this.userSubscriptionCount === 0) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "user.unsubscribe",
        payload: {},
      });
    }
  }

  on<T extends BackendMessageType>(type: T, handler: MessageHandler<T>) {
    const handlers = this.handlers.get(type) ?? new Set();
    handlers.add(handler as MessageHandler<BackendMessageType>);
    this.handlers.set(type, handlers);
    return () => this.off(type, handler);
  }

  off<T extends BackendMessageType>(type: T, handler: MessageHandler<T>) {
    const handlers = this.handlers.get(type);
    if (!handlers) return;
    handlers.delete(handler as MessageHandler<BackendMessageType>);
    if (!handlers.size) {
      this.handlers.delete(type);
    }
  }

  onConnectionStatus(handler: (status: WebSocketStatus) => void) {
    this.statusHandlers.add(handler);
    return () => this.statusHandlers.delete(handler);
  }

  onRawSessionEvent(handler: (event: RawSessionEvent) => void) {
    this.rawSessionEventHandlers.add(handler);
    return () => {
      this.rawSessionEventHandlers.delete(handler);
    };
  }

  private debugNotification(action: BackendMessageType, payload: unknown, handlerCount: number) {
    if (!isDebug() || DISPATCH_LOG_DENYLIST.has(action)) return;
    const payloadSessionId = (payload as { session_id?: string } | undefined)?.session_id;
    debugDispatch("notification", {
      action,
      sessionId: payloadSessionId ?? null,
      handlers: handlerCount,
    });
  }

  private handleParsedMessage(value: unknown) {
    if (isRawSessionEvent(value)) {
      const disposition = orderedCoreDisposition(value);
      this.handleCoreSessionEvent(value, disposition);
      this.rawSessionEventHandlers.forEach((handler) => handler(value));
      return;
    }
    if (this.handleMalformedSessionEnvelope(value)) return;
    if (!value || typeof value !== "object" || !("type" in value)) return;
    const rawMessage = value as RawWebSocketMessage;
    if (this.handleRequestResult(rawMessage)) return;
    const message = value as BackendMessageMap[BackendMessageType];
    if (message.type !== "notification") return;
    const action = message.action;
    if (!action) return;
    const handlers = this.handlers.get(action);
    this.debugNotification(action, message.payload, handlers?.size ?? 0);
    if (handlers) {
      handlers.forEach((handler) => handler(message));
    }
    dispatchToPluginWsHandlers(action, message.payload);
  }
  /**
   * A frame that is session.event-shaped but fails the strict envelope check
   * (bad protocol version, missing identity fields, malformed payload) can
   * never be a valid ordered row. If it targets a live core stream at the
   * expected next sequence, run the same durable poison recovery as a poison
   * disposition so the stream rebinds to the authoritative watermark instead
   * of stalling silently on a hole; the malformed frame itself is never
   * dispatched to consumers.
   */
  private handleMalformedSessionEnvelope(value: unknown): boolean {
    if (!value || typeof value !== "object") return false;
    const candidate = value as {
      type?: unknown;
      session_id?: unknown;
      task_id?: unknown;
      sequence?: unknown;
    };
    if (candidate.type !== "session.event") return false;
    const sessionId = candidate.session_id;
    const sequence = candidate.sequence;
    const taskId = candidate.task_id;
    const stream =
      typeof sessionId === "string" && typeof sequence === "number"
        ? this.coreSessionStreams.get(sessionId)
        : undefined;
    if (stream && sequence === stream.lastSeenSequence + 1) {
      if (isDebug()) {
        console.warn(
          `[ws] malformed ordered session.event frame for "${sessionId}" at ${sequence}`,
        );
      }
      this.recoverCoreSessionPoison(sessionId as string, stream);
    }
    if (
      typeof sessionId === "string" &&
      Number.isSafeInteger(sequence) &&
      (sequence as number) > 0
    ) {
      this.notifyMalformedSessionEvent(
        sessionId,
        sequence as number,
        typeof taskId === "string" ? taskId : null,
      );
    }
    return true;
  }
  private notifyMalformedSessionEvent(sessionId: string, sequence: number, taskId: string | null) {
    const poison: RawSessionEvent = {
      type: "session.event",
      protocol_version: 1,
      event_type: "session.event.poison",
      session_id: sessionId,
      task_id: taskId,
      sequence,
      event_id: `poison:${sessionId}:${sequence}`,
      payload: {
        type: "session.event.poison",
        session_id: sessionId,
        task_id: taskId,
      },
    };
    this.rawSessionEventHandlers.forEach((handler) => handler(poison));
  }

  private handleRequestResult(message: RawWebSocketMessage): boolean {
    if (message.type !== "response" && message.type !== "error") return false;
    if (typeof message.id !== "string") return false;
    if (message.type === "response") {
      if (isDebug()) debugDispatch("response", { id: message.id });
      this.resolvePendingRequest(message.id, message.payload);
    } else {
      if (isDebug()) debugDispatch("error-response", { id: message.id });
      this.rejectPendingRequest(message.id, message.payload);
    }
    return true;
  }

  private resolvePendingRequest(msgId: string, payload: unknown) {
    const pending = this.pendingRequests.get(msgId);
    if (!pending) return;
    clearTimeout(pending.timeout);
    this.pendingRequests.delete(msgId);
    pending.resolve(payload);
  }

  // i18n-exempt: transport/API diagnostic. Callers branch on the error code and
  // render translated copy; this text only ever appears in a console or as an
  // interpolated English diagnostic (see docs/i18n.md on interpolated values).
  private rejectPendingRequest(msgId: string, payload: unknown) {
    const pending = this.pendingRequests.get(msgId);
    if (!pending) return;
    clearTimeout(pending.timeout);
    this.pendingRequests.delete(msgId);
    pending.reject(toWebSocketRequestError(payload));
  }

  private handleDisconnect(event: CloseEvent) {
    this.setStatus("disconnected");
    const shouldRetainSessionReadiness =
      !this.intentionalClose &&
      this.reconnectOptions.enabled &&
      this.reconnectAttempts < this.reconnectOptions.maxAttempts;
    this.resetSessionSubscriptionReadiness(
      new Error(WEBSOCKET_CONNECTION_CLOSED_ERROR),
      shouldRetainSessionReadiness,
    );

    // Don't reconnect if this was an intentional close
    if (this.intentionalClose) {
      return;
    }

    // Don't reconnect if reconnect is disabled
    if (!this.reconnectOptions.enabled) {
      return;
    }

    // Don't reconnect if we've exceeded max attempts
    if (this.reconnectAttempts >= this.reconnectOptions.maxAttempts) {
      console.warn(
        `WebSocket max reconnect attempts (${this.reconnectOptions.maxAttempts}) reached`,
      );
      this.setStatus("error");
      this.cleanupPendingRequests();
      return;
    }

    // Calculate delay with exponential backoff
    const delay = Math.min(
      this.reconnectOptions.initialDelay *
        Math.pow(this.reconnectOptions.backoffMultiplier, this.reconnectAttempts),
      this.reconnectOptions.maxDelay,
    );

    this.reconnectAttempts++;
    this.setStatus("reconnecting");

    console.log(
      `WebSocket disconnected (code: ${event.code}, reason: ${event.reason || "none"}). ` +
        `Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.reconnectOptions.maxAttempts})...`,
    );

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private cleanupPendingRequests() {
    // Reject all pending requests
    this.pendingRequests.forEach(({ reject, timeout }) => {
      clearTimeout(timeout);
      reject(new Error(WEBSOCKET_CONNECTION_CLOSED_ERROR));
    });
    this.pendingRequests.clear();
  }

  private getOrCreateSessionSubscriptionReadiness(
    sessionId: string,
    forceNewAfterSettled = false,
  ): SessionSubscriptionReadiness {
    const existing = this.sessionSubscriptionReadiness.get(sessionId);
    if (existing && !(forceNewAfterSettled && existing.settled)) return existing;

    let resolve!: () => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<void>((resolvePromise, rejectPromise) => {
      resolve = resolvePromise;
      reject = rejectPromise;
    });
    const readiness: SessionSubscriptionReadiness = {
      promise,
      resolve,
      reject,
      requestStarted: false,
      settled: false,
    };
    // subscribeSession() consumers do not await readiness, so handle failures
    // while returning the original promise to readiness-aware consumers.
    void promise.catch(() => undefined);
    this.sessionSubscriptionReadiness.set(sessionId, readiness);
    return readiness;
  }

  private startSessionSubscription(sessionId: string, readiness: SessionSubscriptionReadiness) {
    if (readiness.requestStarted) return;
    readiness.requestStarted = true;
    const stream = this.getOrCreateCoreSessionStream(sessionId);
    stream.ready = false;
    stream.pendingEvents.length = 0;
    const legacySubscription = this.request("session.subscribe", { session_id: sessionId });
    const orderedSubscription = this.request<unknown>("session.subscribe", {
      session_id: sessionId,
      consumer_kind: "core",
      wire_id: stream.wireId,
      ...(stream.resumeToken || stream.lastSeenSequence > 0
        ? { last_seen_sequence: stream.lastSeenSequence }
        : {}),
      ...(stream.resumeToken ? { resume_token: stream.resumeToken } : {}),
    }).then((response) => {
      const validated = validateOrderedSubscribeResponse(response, sessionId, stream.wireId);
      if (validated.result !== "replay") {
        stream.lastSeenSequence = Math.max(stream.lastSeenSequence, validated.eventWatermark);
      }
      stream.resumeToken = validated.resumeToken;
      stream.ready = true;
      this.drainCoreSessionEvents(sessionId, stream);
    });
    void Promise.all([legacySubscription, orderedSubscription])
      .then(() => {
        if (this.sessionSubscriptionReadiness.get(sessionId) !== readiness) return;
        readiness.settled = true;
        readiness.resolve();
      })
      .catch((error: unknown) => {
        stream.ready = false;
        stream.pendingEvents.length = 0;
        if (this.sessionSubscriptionReadiness.get(sessionId) === readiness) {
          this.sessionSubscriptionReadiness.delete(sessionId);
        }
        readiness.settled = true;
        readiness.reject(error);
      });
  }

  private getOrCreateCoreSessionStream(sessionId: string): CoreSessionStream {
    const current = this.coreSessionStreams.get(sessionId);
    if (current) return current;
    const stream = {
      wireId: `core:web:${generateUUID()}`,
      lastSeenSequence: 0,
      ready: false,
      pendingEvents: [],
    };
    this.coreSessionStreams.set(sessionId, stream);
    return stream;
  }

  private handleCoreSessionEvent(
    event: RawSessionEvent,
    disposition = orderedCoreDisposition(event),
  ) {
    const stream = this.coreSessionStreams.get(event.session_id);
    if (!stream) return;
    if (!stream.ready) {
      stream.pendingEvents.push(event);
      return;
    }
    if (stream.pendingEvents.length > 0) {
      stream.pendingEvents.push(event);
      this.drainCoreSessionEvents(event.session_id, stream);
      return;
    }
    this.processCoreSessionEvent(event, stream, disposition);
    this.drainCoreSessionEvents(event.session_id, stream);
  }

  private drainCoreSessionEvents(sessionId: string, stream: CoreSessionStream) {
    if (!stream.ready || stream.pendingEvents.length === 0) return;
    stream.pendingEvents.sort((left, right) => left.sequence - right.sequence);
    while (stream.pendingEvents.length > 0) {
      const event = stream.pendingEvents[0];
      if (!event) return;
      if (event.sequence > stream.lastSeenSequence + 1) {
        stream.pendingEvents.length = 0;
        this.recoverCoreSessionPoison(sessionId, stream);
        return;
      }
      stream.pendingEvents.shift();
      this.processCoreSessionEvent(event, stream);
    }
  }

  private processCoreSessionEvent(
    event: RawSessionEvent,
    stream: CoreSessionStream,
    disposition = orderedCoreDisposition(event),
  ) {
    if (event.sequence <= stream.lastSeenSequence) {
      void this.acknowledgeCoreSessionEvent(
        event.session_id,
        stream,
        stream.lastSeenSequence,
      ).catch(() => this.recoverCoreSessionPoison(event.session_id, stream));
      return;
    }
    if (event.sequence !== stream.lastSeenSequence + 1) {
      this.recoverCoreSessionPoison(event.session_id, stream);
      return;
    }
    if (disposition === "poison") {
      this.recoverCoreSessionPoison(event.session_id, stream);
      return;
    }
    if (disposition === "project") {
      const action = orderedCoreAction(event.event_type);
      if (!action) return;
      const payload = projectCoreSessionPayload(event);
      const message = {
        type: "notification",
        action,
        payload,
      } as BackendMessageMap[BackendMessageType];
      const handlers = this.handlers.get(action);
      this.debugNotification(action, payload, handlers?.size ?? 0);
      handlers?.forEach((handler) => handler(message));
      dispatchToPluginWsHandlers(action, payload);
    }
    stream.lastSeenSequence = event.sequence;
    void this.acknowledgeCoreSessionEvent(event.session_id, stream, event.sequence).catch(() =>
      this.recoverCoreSessionPoison(event.session_id, stream),
    );
  }

  private recoverCoreSessionPoison(sessionId: string, stream: CoreSessionStream) {
    if (stream.poisonRecovery) return;
    stream.poisonRecovery = this.request("session.subscribe", {
      session_id: sessionId,
      consumer_kind: "core",
      wire_id: stream.wireId,
      last_seen_sequence: stream.lastSeenSequence,
      ...(stream.resumeToken ? { resume_token: stream.resumeToken } : {}),
      replace_cursor: true,
    })
      .then((response) => {
        const validated = validateOrderedSubscribeResponse(response, sessionId, stream.wireId);
        if (validated.result !== "replay") {
          stream.lastSeenSequence = Math.max(stream.lastSeenSequence, validated.eventWatermark);
        }
        stream.resumeToken = validated.resumeToken;
      })
      .catch(() => undefined)
      .finally(() => {
        stream.poisonRecovery = undefined;
      });
  }

  private async acknowledgeCoreSessionEvent(
    sessionId: string,
    stream: CoreSessionStream,
    sequence: number,
  ) {
    const response = await this.request<unknown>("session.ack", {
      session_id: sessionId,
      consumer_kind: "core",
      wire_id: stream.wireId,
      sequence,
      resume_token: stream.resumeToken,
    });
    stream.resumeToken = validateOrderedAckResponse(response, sessionId, stream.wireId, sequence);
  }
  private cancelSessionSubscriptionReadiness(sessionId: string) {
    const readiness = this.sessionSubscriptionReadiness.get(sessionId);
    if (!readiness) return;
    this.sessionSubscriptionReadiness.delete(sessionId);
    if (!readiness.settled) {
      readiness.settled = true;
      readiness.reject(new Error("Session subscription released"));
    }
  }

  private resetSessionSubscriptionReadiness(error: Error, retainActiveSubscriptions = false) {
    const readinessEntries = [...this.sessionSubscriptionReadiness.entries()];
    this.sessionSubscriptionReadiness.clear();
    for (const [, readiness] of readinessEntries) {
      if (readiness.settled) continue;
      readiness.settled = true;
      readiness.reject(error);
    }
    if (retainActiveSubscriptions) {
      this.sessionSubscriptions.forEach((_, id) =>
        this.getOrCreateSessionSubscriptionReadiness(id),
      );
    }
  }

  private resubscribe() {
    // Re-subscribe to all tasks after reconnection
    this.subscriptions.forEach((_count, taskId) => {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "task.subscribe",
        payload: { task_id: taskId },
      });
    });
    this.sessionSubscriptions.forEach((_count, sessionId) => {
      const readiness = this.getOrCreateSessionSubscriptionReadiness(sessionId);
      this.startSessionSubscription(sessionId, readiness);
    });
    this.sessionFocusCounts.forEach((_count, sessionId) => {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "session.focus",
        payload: { session_id: sessionId },
      });
    });
    this.runSubscriptions.forEach((_count, runId) => {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "run.subscribe",
        payload: { run_id: runId },
      });
    });
    if (this.userSubscriptionCount > 0) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "user.subscribe",
        payload: {},
      });
    }
    if (this.systemMetricsSubscriptionCount > 0) {
      this.send({
        id: generateUUID(),
        type: "request",
        action: "system.metrics.subscribe",
        payload: {},
      });
    }
  }

  private flushQueue() {
    if (!this.socket || this.status !== "connected") return;
    this.pendingQueue.forEach((data) => this.socket?.send(data));
    this.pendingQueue = [];
  }

  private setStatus(status: WebSocketStatus) {
    this.status = status;
    this.onStatusChange?.(status);
    this.statusHandlers.forEach((handler) => handler(status));
  }
}
