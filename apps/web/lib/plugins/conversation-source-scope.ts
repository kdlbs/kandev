import { generateUUID } from "@/lib/uuid";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { RawSessionEvent } from "@/lib/ws/client";
import {
  fetchConversationBinding,
  pluginConversationUrl,
  type Binding,
  type ConversationScope,
  type OrderedReady,
  type SnapshotKind,
} from "./conversation-scope";
import {
  reconcileConversationChange,
  type ConversationChange,
  type ConversationChangeOperation,
  type ConversationReconciliationState,
} from "./conversation-reconciliation";

type Listener = (event: RawSessionEvent) => boolean;
type RebindListener = () => void | Promise<void>;

type SourceSubscribeResponse = {
  success: true;
  protocol_version: 2;
  scope_id: string;
  session_id: string;
  epoch: string;
  revision: string;
};

type SourceFailure = {
  success: false;
  error?: { code?: string; message?: string; retryable?: boolean };
};

function sourceError(response: SourceFailure): Error {
  // i18n-exempt: transport/API diagnostic. The caller renders the structured
  // error code through translated UI copy.
  const error = new Error(response.error?.message ?? "Conversation subscription failed");
  Object.assign(error, {
    code: response.error?.code ?? "upstream_failure",
    retryable: response.error?.retryable ?? true,
  });
  return error;
}

function validRevision(value: unknown): value is string {
  return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value);
}

function compareRevision(left: string, right: string): number {
  const leftValue = BigInt(left);
  const rightValue = BigInt(right);
  if (leftValue < rightValue) return -1;
  if (leftValue > rightValue) return 1;
  return 0;
}

function snapshotKindForOperation(operation: ConversationChangeOperation): SnapshotKind {
  return operation.entity === "message" ? "messages" : "turns";
}

function operationTaskID(operation: ConversationChangeOperation, fallback: string): string {
  const value = operation.entity === "message" ? operation.message?.taskId : operation.turn?.taskId;
  return typeof value === "string" && value !== "" ? value : fallback;
}

function operationEventType(operation: ConversationChangeOperation): string {
  if (operation.entity === "message") {
    return operation.kind === "remove" ? "message.deleted" : "message.added";
  }
  return operation.kind === "remove" ? "session.turn.removed" : "session.turn.started";
}

function sourceOperationEvent(
  operation: ConversationChangeOperation,
  sessionID: string,
  taskID: string,
  revision: string,
  index: number,
): RawSessionEvent {
  const eventType = operationEventType(operation);
  const operationTaskId = operationTaskID(operation, taskID);
  const payload: Record<string, unknown> = {
    type: eventType,
    session_id: sessionID,
    task_id: operationTaskId,
  };
  if (operation.entity === "message") {
    payload.message_id = operation.id;
    if (operation.kind === "upsert" && operation.message) {
      const message = operation.message;
      payload.author_type = message.authorType;
      payload.content = message.content;
      payload.message_type = message.type;
      payload.created_at = message.createdAt;
      payload.updated_at = message.updatedAt;
      payload.turn_id = message.turnId;
      payload.prompt_index = message.promptIndex;
      payload.sender_task_id = message.senderTaskId;
    }
  } else {
    payload.id = operation.id;
    if (operation.kind === "upsert" && operation.turn) {
      const turn = operation.turn;
      payload.started_at = turn.startedAt;
      payload.completed_at = turn.completedAt;
      payload.updated_at = turn.updatedAt;
    }
  }
  const numericRevision = Number(revision);
  return {
    type: "session.event",
    protocol_version: 1,
    event_type: eventType,
    session_id: sessionID,
    task_id: operationTaskId,
    sequence: Number.isSafeInteger(numericRevision) ? numericRevision : index + 1,
    event_id: `conversation:${sessionID}:${revision}:${operation.entity}:${operation.id}`,
    payload,
  };
}

export class SourceConversationScope implements ConversationScope {
  readonly source = true;
  readonly signal: AbortSignal;
  private readonly scopeID = `plugin-conversation:${generateUUID()}`;
  private readonly listeners = new Map<Listener, { kind: SnapshotKind; key: string }>();
  private readonly rebindListeners = new Set<RebindListener>();
  private readonly committedSnapshots = new Set<string>();
  private readonly pendingChanges: ConversationChange[] = [];
  private bindingPromise: Promise<Binding> | null = null;
  private readyPromise: Promise<OrderedReady> | null = null;
  private recoveryPromise: Promise<void> | null = null;
  private recovering = false;
  private terminal = false;
  private revisionCheckTimer: ReturnType<typeof setTimeout> | undefined;
  private closed = false;
  private state: ConversationReconciliationState = { appliedRevision: "0", epoch: "" };

  constructor(
    readonly pluginId: string,
    readonly taskId: string,
    readonly sessionId: string | null,
    private readonly controller: AbortController,
  ) {
    this.signal = controller.signal;
  }

  ready(): Promise<OrderedReady> {
    const pending = (this.readyPromise ??= this.initializeReady());
    return pending
      .then(async (current) => {
        const binding = await this.getBinding();
        if (binding.bindingToken === current.bindingToken) return current;
        if (binding.generation !== current.generation) return this.subscribeSource(binding);
        const next = { ...current, ...binding };
        this.readyPromise = Promise.resolve(next);
        return next;
      })
      .catch((error: unknown) => {
        if (this.readyPromise === pending) this.readyPromise = null;
        throw error;
      });
  }

  subscribe(listener: Listener, kind: SnapshotKind, snapshotKey: string): () => void {
    this.listeners.set(listener, { kind, key: snapshotKey });
    return () => {
      this.listeners.delete(listener);
      this.drainPending();
    };
  }

  async renewContinuation(cursor: string): Promise<{
    cursor: string;
    binding: OrderedReady;
  }> {
    return { cursor, binding: await this.ready() };
  }

  subscribeRebind(listener: RebindListener): () => void {
    this.rebindListeners.add(listener);
    return () => this.rebindListeners.delete(listener);
  }

  isTerminal(): boolean {
    return this.terminal;
  }

  invalidateSnapshot(kind: SnapshotKind, snapshotKey = "") {
    this.committedSnapshots.delete(`${kind}:${snapshotKey}`);
  }

  commitSnapshot(kind: SnapshotKind, snapshotKey = "") {
    if (this.closed) return;
    this.committedSnapshots.add(`${kind}:${snapshotKey}`);
    this.drainPending();
  }

  setSourceSnapshot(epoch: string | undefined, revision: string | undefined) {
    if (this.closed || !validRevision(revision)) return;
    const nextEpoch = epoch ?? this.state.epoch;
    if (nextEpoch === "") return;
    if (nextEpoch !== this.state.epoch) {
      this.pendingChanges.length = 0;
      this.recovering = false;
      this.state = { epoch: nextEpoch, appliedRevision: revision };
    } else if (compareRevision(revision, this.state.appliedRevision) > 0) {
      this.state = { ...this.state, appliedRevision: revision };
      this.recovering = false;
    }
    this.discardCoveredChanges(this.state.appliedRevision);
    this.updateReadyState();
    this.drainPending();
  }

  acceptChange(value: unknown) {
    if (this.closed || this.terminal || !this.sessionId) return;
    const change = value as Partial<ConversationChange>;
    if (change.session_id !== this.sessionId || change.scope_id !== this.scopeID) return;
    if (change.terminal) {
      this.acceptTerminalRemoval();
      return;
    }
    if (change.check) {
      this.checkRevision(change);
      return;
    }
    if (this.recovering || this.shouldBuffer(change)) {
      this.pendingChanges.push(value as ConversationChange);
      return;
    }
    this.applyChange(value);
  }

  accept(_event: RawSessionEvent): void {
    // Source scopes consume v2 changes and terminal removal notifications separately.
  }

  acceptTerminalRemoval(): void {
    if (this.closed || this.terminal || !this.sessionId) return;
    this.terminal = true;
    clearTimeout(this.revisionCheckTimer);
    this.pendingChanges.length = 0;
    const event: RawSessionEvent = {
      type: "session.event",
      protocol_version: 1,
      event_type: "session.removed",
      session_id: this.sessionId,
      task_id: null,
      sequence: 0,
      event_id: `conversation:removed:${this.sessionId}`,
      payload: {
        type: "session.removed",
        session_id: this.sessionId,
      },
    };
    for (const listener of this.listeners.keys()) {
      try {
        listener(event);
      } catch {
        // A terminal notification must not prevent other consumers from closing.
      }
    }
  }

  reconnect() {
    if (this.closed || this.terminal || !this.sessionId || !this.readyPromise) return;
    void this.resubscribe().catch(() => undefined);
  }

  close() {
    this.closed = true;
    clearTimeout(this.revisionCheckTimer);
    this.pendingChanges.length = 0;
    this.listeners.clear();
    this.rebindListeners.clear();
    const ready = this.readyPromise;
    if (!ready || !this.sessionId) return;
    void ready
      .then((current) =>
        getWebSocketClient()?.request("session.conversation.unsubscribe", {
          scope_id: this.scopeID,
          session_id: this.sessionId,
          consumer_kind: "plugin",
          plugin_id: this.pluginId,
          generation: current.generation,
        }),
      )
      .catch(() => undefined);
  }

  private checkRevision(change: Partial<ConversationChange>) {
    if (
      !validRevision(change.revision) ||
      change.epoch !== this.state.epoch ||
      compareRevision(change.revision, this.state.appliedRevision) < 0
    ) {
      this.beginRecovery();
      return;
    }
    if (change.revision === this.state.appliedRevision || this.revisionCheckTimer !== undefined)
      return;
    const observed = change.revision;
    this.revisionCheckTimer = setTimeout(() => {
      this.revisionCheckTimer = undefined;
      if (
        !this.closed &&
        !this.terminal &&
        compareRevision(observed, this.state.appliedRevision) > 0
      ) {
        this.beginRecovery();
      }
    }, 1000);
  }

  private async initializeReady(): Promise<OrderedReady> {
    const binding = await this.getBinding();
    if (!this.sessionId) {
      return {
        ...binding,
        source: true,
        snapshotToken: "",
        snapshotExpiresAt: binding.expiresAt,
        resumeToken: "",
        consumerId: this.scopeID,
        watermark: 0,
      };
    }
    return this.subscribeSource(binding);
  }

  private async getBinding(): Promise<Binding> {
    let pending = (this.bindingPromise ??= fetchConversationBinding(this.pluginId, this.signal));
    try {
      const current = await pending;
      if (new Date(current.expiresAt).getTime() - Date.now() > 120_000) return current;
      if (this.bindingPromise === pending) {
        this.bindingPromise = fetchConversationBinding(this.pluginId, this.signal);
      }
      pending = this.bindingPromise!;
      return await pending;
    } catch (error) {
      if (this.bindingPromise === pending) this.bindingPromise = null;
      throw error;
    }
  }

  private async subscribeSource(binding: Binding, notifyOnChange = true): Promise<OrderedReady> {
    const client = getWebSocketClient();
    // i18n-exempt: transport/API diagnostic. The caller renders the
    // structured error code through translated UI copy.
    if (!client) throw new Error("WebSocket unavailable");
    const response = await client.request<SourceSubscribeResponse | SourceFailure>(
      "session.conversation.subscribe",
      {
        scope_id: this.scopeID,
        session_id: this.sessionId,
        consumer_kind: "plugin",
        plugin_id: this.pluginId,
        generation: binding.generation,
        binding_token: binding.bindingToken,
        task_id: this.taskId,
      },
    );
    if (!response.success) throw sourceError(response);
    if (
      response.protocol_version !== 2 ||
      response.session_id !== this.sessionId ||
      response.scope_id !== this.scopeID ||
      !response.epoch ||
      !validRevision(response.revision)
    ) {
      // i18n-exempt: transport/API diagnostic. The caller renders the
      // structured error code through translated UI copy.
      throw new Error("Invalid conversation subscription response");
    }
    const changed =
      this.state.epoch !== "" &&
      (this.state.epoch !== response.epoch ||
        compareRevision(response.revision, this.state.appliedRevision) > 0);
    this.state = { epoch: response.epoch, appliedRevision: response.revision };
    this.recovering = false;
    const next: OrderedReady = {
      ...binding,
      source: true,
      snapshotToken: "",
      snapshotExpiresAt: binding.expiresAt,
      resumeToken: "",
      consumerId: this.scopeID,
      watermark: 0,
      epoch: response.epoch,
      revision: response.revision,
    };
    this.readyPromise = Promise.resolve(next);
    if (changed && notifyOnChange) await this.notifyRebindListeners();
    return next;
  }

  private async resubscribe(notifyOnChange = true) {
    const binding = await this.getBinding();
    await this.subscribeSource(binding, notifyOnChange);
  }

  private updateReadyState() {
    if (!this.readyPromise) return;
    void this.readyPromise.then((ready) => {
      if (this.readyPromise) {
        this.readyPromise = Promise.resolve({
          ...ready,
          epoch: this.state.epoch,
          revision: this.state.appliedRevision,
        });
      }
    });
  }

  private shouldBuffer(value: Partial<ConversationChange>): boolean {
    if (!Array.isArray(value.operations) || value.operations.length === 0) return false;
    return value.operations.some((operation) => {
      if (!operation || (operation.kind !== "upsert" && operation.kind !== "remove")) return true;
      const kind = snapshotKindForOperation(operation as ConversationChangeOperation);
      return [...this.listeners.values()]
        .filter((listener) => listener.kind === kind)
        .some((listener) => !this.committedSnapshots.has(`${kind}:${listener.key}`));
    });
  }

  private applyChange(value: unknown) {
    const result = reconcileConversationChange(this.state, value);
    if (result.kind !== "applied") {
      if (result.kind === "recover") {
        this.pendingChanges.push(value as ConversationChange);
        this.beginRecovery();
      }
      return;
    }
    this.state = result.state;
    this.updateReadyState();
    const change = value as ConversationChange;
    result.operations.forEach((operation, index) => {
      const event = sourceOperationEvent(
        operation,
        change.session_id,
        this.taskId,
        change.revision,
        index,
      );
      for (const listener of this.listeners.keys()) {
        try {
          if (!listener(event)) {
            this.beginRecovery();
            return;
          }
        } catch {
          this.beginRecovery();
          return;
        }
      }
    });
  }

  private discardCoveredChanges(revision: string) {
    this.pendingChanges.splice(
      0,
      this.pendingChanges.length,
      ...this.pendingChanges.filter(
        (change) =>
          validRevision(change.revision) && compareRevision(change.revision, revision) > 0,
      ),
    );
  }

  private drainPending() {
    if (this.closed || this.recovering || this.pendingChanges.length === 0) return;
    const pending = this.pendingChanges.splice(0);
    for (const change of pending) {
      if (this.shouldBuffer(change)) {
        this.pendingChanges.push(change);
        continue;
      }
      this.applyChange(change);
      if (this.recovering) return;
    }
  }

  private beginRecovery() {
    if (this.recovering || this.closed) return;
    this.recovering = true;
    this.recoveryPromise ??= this.resubscribe(false)
      .catch(() => undefined)
      .then(() => this.notifyRebindListeners())
      .catch(() => undefined)
      .finally(() => {
        this.recoveryPromise = null;
      });
  }

  private async notifyRebindListeners() {
    await Promise.all(
      [...this.rebindListeners].map((listener) => Promise.resolve().then(() => listener())),
    );
  }
}

export function sourceConversationMessageUrl(
  pluginId: string,
  sessionId: string,
  query: URLSearchParams,
): string {
  return pluginConversationUrl(
    pluginId,
    `/conversation/v2/task-sessions/${encodeURIComponent(sessionId)}/messages?${query}`,
  );
}
