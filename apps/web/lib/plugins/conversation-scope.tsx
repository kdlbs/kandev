/* eslint-disable max-lines -- The scope owns transport, ordering, and snapshot lifecycle state. */
"use client";

import * as React from "react";
import { getBackendConfig } from "@/lib/config";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { generateUUID } from "@/lib/uuid";
import { isRawSessionEvent } from "@/lib/ws/ordered-session-events";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { RawSessionEvent } from "@/lib/ws/client";
import type { PluginConversationError } from "./types";
import { SourceConversationScope } from "./conversation-source-scope";

export type OrderedReady = {
  bindingToken: string;
  generation: number;
  expiresAt: string;
  snapshotExpiresAt: string;
  snapshotToken: string;
  resumeToken: string;
  consumerId: string;
  watermark: number;
  source?: boolean;
  epoch?: string;
  revision?: string;
};

export type Binding = Pick<OrderedReady, "bindingToken" | "generation" | "expiresAt">;
export type SnapshotKind = "messages" | "turns";
type ConversationEventListener = (event: RawSessionEvent) => boolean;
type SnapshotKey = string;
type OrderedSubscribeAck =
  | {
      success: true;
      snapshot_token: string;
      resume_token: string;
      consumer_id: string;
      event_watermark: number;
      expires_at: string;
      result: "fresh" | "replay" | "invalid_resume";
      replay_from?: number;
    }
  | {
      success: false;
      error:
        | PluginConversationError
        | { code: "session_removed"; message: string; retryable: false };
    };
type ContinuationRenewal = {
  cursor: string;
  snapshot_token: string;
  expires_at: string;
};
type ErrorEnvelope = { error?: PluginConversationError };
type RebindListener = () => void | Promise<void>;
type ContinuationResult = { cursor: string; binding: OrderedReady; recovered?: boolean };

export type ConversationScope = {
  pluginId: string;
  taskId: string;
  sessionId: string | null;
  managedConversationToken?: string;
  signal: AbortSignal;
  ready(): Promise<OrderedReady>;
  renewContinuation(cursor: string, queryIdentity?: string): Promise<ContinuationResult>;
  subscribe(
    listener: ConversationEventListener,
    kind: SnapshotKind,
    snapshotKey: SnapshotKey,
  ): () => void;
  subscribeRebind(listener: RebindListener): () => void;
  isTerminal(): boolean;
  commitSnapshot(
    kind: SnapshotKind,
    snapshotKey?: SnapshotKey,
    epoch?: string,
    revision?: string,
  ): void;
  setSourceSnapshot?(epoch: string | undefined, revision: string | undefined): void;
  acceptChange?(value: unknown): void;
  acceptTerminalRemoval?(): void;
  readonly source?: boolean;
  invalidateSnapshot(kind: SnapshotKind, snapshotKey?: SnapshotKey): void;
  reconnect(): void;
  accept(event: RawSessionEvent): void;
  close(): void;
};

export const ConversationScopeContext = React.createContext<ConversationScope | null>(null);

export function pluginConversationUrl(pluginId: string, path: string): string {
  const { apiBaseUrl } = getBackendConfig();
  return `${apiBaseUrl}/api/plugins/${encodeURIComponent(pluginId)}${path}`;
}

export async function parseConversationResponse<T>(response: Response): Promise<T> {
  if (response.ok) return (await response.json()) as T;
  const body = (await response.json().catch(() => ({}))) as ErrorEnvelope;
  throw (
    body.error ?? {
      code: "upstream_failure",
      // i18n-exempt: plugin-facing transport diagnostic; plugins render their own localized error UI.
      message: `Conversation request failed with status ${response.status}`,
      retryable: response.status >= 500,
    }
  );
}

export async function fetchConversationBinding(
  pluginId: string,
  signal: AbortSignal,
): Promise<Binding> {
  const response = await fetch(pluginConversationUrl(pluginId, "/conversation/binding"), {
    credentials: "include",
    cache: "no-store",
    signal,
  });
  return parseConversationResponse<Binding>(response);
}

// i18n-exempt: managed conversation dispatch protocol statuses, not user-facing copy.
const acceptedManagedDispatchStatuses = new Set(["started", "sent", "duplicate_occurrence"]);

export async function dispatchManagedConversation({
  pluginId,
  sessionId,
  workspaceId,
  content,
  occurrenceKey,
  bindingToken,
}: {
  pluginId: string;
  sessionId: string;
  workspaceId: string;
  content: string;
  occurrenceKey: string;
  bindingToken: string;
}): Promise<void> {
  const response = await fetch(
    pluginConversationUrl(
      pluginId,
      `/conversation/managed/${encodeURIComponent(sessionId)}/dispatch?workspace_id=${encodeURIComponent(workspaceId)}`,
    ),
    {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-Kandev-Plugin-Binding": bindingToken,
      },
      body: JSON.stringify({ content, occurrenceKey }),
    },
  );
  const delivery = (await response.json().catch(() => null)) as { status?: string } | null;
  if (!response.ok || !delivery?.status || !acceptedManagedDispatchStatuses.has(delivery.status)) {
    // i18n-exempt: internal transport error, rendered through localized component copy.
    throw new Error("managed conversation dispatch failed");
  }
}

const SESSION_REMOVED_EVENT = "session.removed";
const SESSION_SUBSCRIBE_ACTION = "session.subscribe";

const ORDERED_EVENT_TYPES: Record<string, true> = {
  "message.added": true,
  "message.updated": true,
  "message.deleted": true,
  "session.turn.started": true,
  "session.turn.completed": true,
  "session.turn.removed": true,
  [SESSION_REMOVED_EVENT]: true,
};

function isNonEmptyPayloadString(payload: Record<string, unknown>, key: string): boolean {
  return typeof payload[key] === "string" && (payload[key] as string).length > 0;
}

function isValidPayloadTime(payload: Record<string, unknown>, key: string): boolean {
  if (!isNonEmptyPayloadString(payload, key)) return false;
  const value = payload[key];
  return typeof value === "string" && parseTurnTimestamp(value) !== null;
}

function isValidMessagePayload(payload: Record<string, unknown>, updated: boolean): boolean {
  return (
    isNonEmptyPayloadString(payload, "message_id") &&
    (payload.author_type === "user" || payload.author_type === "agent") &&
    typeof payload.content === "string" &&
    isValidPayloadTime(payload, "created_at") &&
    (!updated || isValidPayloadTime(payload, "updated_at"))
  );
}

function isValidTurnPayload(payload: Record<string, unknown>, completed: boolean): boolean {
  return (
    isNonEmptyPayloadString(payload, "id") &&
    isValidPayloadTime(payload, "started_at") &&
    (!completed ||
      (isValidPayloadTime(payload, "completed_at") && isValidPayloadTime(payload, "updated_at")))
  );
}
// eventPayloadValidators mirrors backend ProjectSessionEvent's per-event
// required-field checks. session.removed is intentionally absent: a removal is
// accepted for the selected session regardless of its task_id.
const eventPayloadValidators: Record<string, (payload: Record<string, unknown>) => boolean> = {
  "message.added": (payload) => isValidMessagePayload(payload, false),
  "message.updated": (payload) => isValidMessagePayload(payload, true),
  "message.deleted": (payload) => isNonEmptyPayloadString(payload, "message_id"),
  "session.turn.started": (payload) => isValidTurnPayload(payload, false),
  "session.turn.completed": (payload) => isValidTurnPayload(payload, true),
  "session.turn.removed": (payload) => isNonEmptyPayloadString(payload, "id"),
};

function isCompatibleConversationEvent(event: RawSessionEvent, sessionId: string): boolean {
  if (!isRawSessionEvent(event) || event.session_id !== sessionId) return false;
  if (
    !(event.event_type in ORDERED_EVENT_TYPES) ||
    !event.payload ||
    typeof event.payload !== "object"
  ) {
    return false;
  }
  const payload = event.payload as Record<string, unknown>;
  if (payload.type !== event.event_type || payload.session_id !== sessionId) return false;
  if (event.event_type === SESSION_REMOVED_EVENT) {
    return true;
  }
  // Every non-removal event must match the session's task identity exactly.
  if (event.task_id !== null && payload.task_id !== event.task_id) return false;
  const validator = eventPayloadValidators[event.event_type];
  return validator ? validator(payload) : false;
}

function snapshotKindForEvent(eventType: string): SnapshotKind | null {
  if (eventType.startsWith("message.")) return "messages";
  if (eventType.startsWith("session.turn.")) return "turns";
  return null;
}

class OrderedConversationScope implements ConversationScope {
  readonly signal: AbortSignal;
  private bindingPromise: Promise<Binding> | null = null;
  private readyPromise: Promise<OrderedReady> | null = null;
  private readonly renewalPromises = new Map<string, Promise<ContinuationResult>>();
  private bindingRefreshPromise: Promise<OrderedReady> | null = null;
  private continuationRecoveryPromise: Promise<OrderedReady> | null = null;
  private stateGeneration = 0;
  private currentResumeToken = "";
  private readonly committedSnapshots = new Set<string>();
  private acknowledgedSequence = 0;
  private nextSequence = 1;
  private sequenceBlocked = false;
  private terminal = false;
  private closed = false;
  private ackChain = Promise.resolve();
  private readonly listeners = new Set<ConversationEventListener>();
  private readonly listenerSnapshotKeys = new Map<ConversationEventListener, string>();
  private readonly listenerSnapshotKinds = new Map<ConversationEventListener, SnapshotKind>();
  private readonly rebindListeners = new Set<() => void>();
  private poisonRebindPromise: Promise<void> | null = null;
  private readonly buffered: RawSessionEvent[] = [];
  private readonly pendingBySequence = new Map<number, RawSessionEvent>();
  private consumerId = generateUUID();

  constructor(
    readonly pluginId: string,
    readonly taskId: string,
    readonly sessionId: string | null,
    private readonly controller: AbortController,
    readonly managedConversationToken?: string,
  ) {
    this.signal = controller.signal;
  }

  ready(): Promise<OrderedReady> {
    const pending = (this.readyPromise ??= this.initializeReady());
    return pending
      .then((current) => this.refreshBindingIfNeeded(current))
      .catch((error: unknown) => {
        if (this.readyPromise === pending) this.readyPromise = null;
        throw error;
      });
  }

  async renewContinuation(cursor: string, queryIdentity = ""): Promise<ContinuationResult> {
    const current = await this.ready();
    const snapshotExpiresAt = new Date(current.snapshotExpiresAt).getTime();
    if (snapshotExpiresAt - Date.now() > 2 * 60 * 1000) {
      return { cursor, binding: current };
    }
    if (snapshotExpiresAt <= Date.now()) {
      const binding = await this.recoverExpiredContinuation(current);
      return { cursor, binding, recovered: true };
    }
    const renewalKey = JSON.stringify([queryIdentity, cursor]);
    const existing = this.renewalPromises.get(renewalKey);
    if (existing) return existing;
    const capturedGeneration = this.stateGeneration;
    const pending = fetch(
      pluginConversationUrl(this.pluginId, "/conversation/continuation/renew"),
      {
        method: "POST",
        credentials: "include",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          "X-Kandev-Plugin-Binding": current.bindingToken,
          ...(this.managedConversationToken
            ? { "X-Kandev-Managed-Conversation": this.managedConversationToken }
            : {}),
        },
        body: JSON.stringify({ cursor, snapshot_token: current.snapshotToken }),
        signal: this.signal,
      },
    )
      .then((response) => parseConversationResponse<ContinuationRenewal>(response))
      .then((renewed) => {
        if (this.stateGeneration !== capturedGeneration) {
          return this.ready().then((binding) => ({ cursor, binding, recovered: true }));
        }
        const next = {
          ...current,
          snapshotToken: renewed.snapshot_token,
          snapshotExpiresAt: renewed.expires_at,
        };
        this.readyPromise = Promise.resolve(next);
        return { cursor: renewed.cursor, binding: next };
      })
      .catch((cause: unknown) => {
        if (snapshotExpiresAt <= Date.now()) {
          return this.recoverExpiredContinuation(current).then((binding) => ({
            cursor,
            binding,
            recovered: true,
          }));
        }
        throw cause;
      })
      .finally(() => {
        if (this.renewalPromises.get(renewalKey) === pending) {
          this.renewalPromises.delete(renewalKey);
        }
      });
    this.renewalPromises.set(renewalKey, pending);
    return pending;
  }

  subscribe(listener: ConversationEventListener, kind: SnapshotKind, snapshotKey: SnapshotKey) {
    this.listeners.add(listener);
    this.listenerSnapshotKeys.set(listener, snapshotKey);
    this.listenerSnapshotKinds.set(listener, kind);
    return () => {
      this.listeners.delete(listener);
      this.listenerSnapshotKeys.delete(listener);
      this.listenerSnapshotKinds.delete(listener);
      this.drainBuffered();
    };
  }

  subscribeRebind(listener: RebindListener) {
    this.rebindListeners.add(listener);
    return () => this.rebindListeners.delete(listener);
  }
  isTerminal() {
    return this.terminal;
  }

  invalidateSnapshot(kind: SnapshotKind, snapshotKey = "") {
    if (this.closed) return;
    this.committedSnapshots.delete(`${kind}:${snapshotKey}`);
  }

  commitSnapshot(kind: SnapshotKind, snapshotKey = "") {
    if (this.closed || this.terminal) return;
    this.committedSnapshots.add(`${kind}:${snapshotKey}`);
    this.drainBuffered();
  }

  private drainBuffered() {
    const pending = this.buffered.splice(0).sort((left, right) => left.sequence - right.sequence);
    for (let index = 0; index < pending.length; index += 1) {
      const event = pending[index];
      if (!event) continue;
      if (this.terminal) return;
      if (this.shouldBuffer(event)) {
        this.buffered.push(...pending.slice(index));
        return;
      }
      if (!this.project(event)) {
        this.buffered.unshift(...pending.slice(index + 1));
        this.blockForPoison();
        return;
      }
    }
  }

  accept(event: RawSessionEvent) {
    const sessionId = this.sessionId;
    if (!sessionId || event.session_id !== sessionId) return;
    if (!isRawSessionEvent(event)) {
      this.blockForPoison();
      return;
    }
    if (!this.acceptsScope(event, sessionId) || event.sequence < this.nextSequence) return;
    if (this.sequenceBlocked && event.event_type === SESSION_REMOVED_EVENT) {
      if (isCompatibleConversationEvent(event, sessionId)) this.project(event, false);
      return;
    }
    const existing = this.pendingBySequence.get(event.sequence);
    if (existing) {
      if (existing.event_id !== event.event_id || existing.event_type !== event.event_type) {
        this.blockForPoison();
      }
      return;
    }
    this.pendingBySequence.set(event.sequence, event);
    this.drainPending(sessionId);
  }

  private acceptsScope(event: RawSessionEvent, sessionId: string) {
    return !this.closed && !this.terminal && event.session_id === sessionId;
  }

  private drainPending(sessionId: string) {
    while (!this.sequenceBlocked && !this.terminal) {
      const event = this.pendingBySequence.get(this.nextSequence);
      if (!event) return;
      if (!isCompatibleConversationEvent(event, sessionId)) {
        this.blockForPoison();
        return;
      }
      this.pendingBySequence.delete(this.nextSequence);
      this.nextSequence += 1;
      if (event.event_type === SESSION_REMOVED_EVENT) {
        if (!this.project(event)) this.sequenceBlocked = true;
        return;
      }
      if (this.shouldBuffer(event)) {
        this.buffered.push(event);
        continue;
      }
      if (!this.project(event)) {
        this.sequenceBlocked = true;
        return;
      }
    }
  }

  private shouldBuffer(event: RawSessionEvent) {
    if (this.listeners.size === 0) return true;
    const eventKind = snapshotKindForEvent(event.event_type);
    if (eventKind === null) return false;
    for (const listener of this.listeners) {
      if (this.listenerSnapshotKinds.get(listener) !== eventKind) continue;
      const snapshotKey = this.listenerSnapshotKeys.get(listener) ?? "";
      if (!this.committedSnapshots.has(`${eventKind}:${snapshotKey}`)) return true;
    }
    return false;
  }

  reconnect() {
    if (this.closed || this.terminal || !this.sessionId || !this.readyPromise) return;
    void this.resume().catch(() => undefined);
  }

  close() {
    this.closed = true;
    this.buffered.length = 0;
    this.rebindListeners.clear();
    this.listeners.clear();
    this.listenerSnapshotKeys.clear();
    this.listenerSnapshotKinds.clear();
    this.pendingBySequence.clear();
    if (!this.sessionId || !this.readyPromise) return;
    void this.readyPromise
      .then((ready) =>
        getWebSocketClient()?.request("session.unsubscribe", {
          session_id: this.sessionId,
          consumer_kind: "plugin",
          consumer_id: this.consumerId,
          plugin_id: this.pluginId,
          generation: ready.generation,
        }),
      )
      .catch(() => undefined);
  }

  private getBinding() {
    const pending = (this.bindingPromise ??= fetchConversationBinding(this.pluginId, this.signal));
    return pending.catch((error: unknown) => {
      if (this.bindingPromise === pending) this.bindingPromise = null;
      throw error;
    });
  }

  private managedSubscriptionIdentity(): Record<string, string> {
    return this.managedConversationToken
      ? {
          task_id: this.taskId,
          managed_conversation_token: this.managedConversationToken,
        }
      : {};
  }
  private async initializeReady(): Promise<OrderedReady> {
    const binding = await this.getBinding();
    if (!this.sessionId) {
      return {
        ...binding,
        snapshotToken: "",
        snapshotExpiresAt: binding.expiresAt,
        resumeToken: "",
        consumerId: this.consumerId,
        watermark: 0,
      };
    }
    const client = getWebSocketClient();
    if (!client) {
      // i18n-exempt: plugin-facing transport diagnostic; plugins render their own localized error UI.
      throw new Error("WebSocket unavailable");
    }
    const ack = await client.request<OrderedSubscribeAck>(SESSION_SUBSCRIBE_ACTION, {
      session_id: this.sessionId,
      consumer_kind: "plugin",
      consumer_id: this.consumerId,
      plugin_id: this.pluginId,
      generation: binding.generation,
      ...this.managedSubscriptionIdentity(),
      binding_token: binding.bindingToken,
    });
    if (!ack.success) throw ack.error;
    this.consumerId = ack.consumer_id;
    this.currentResumeToken = ack.resume_token;
    // The subscribe ACK's watermark is the last durable sequence the server
    // already holds; live projection starts at watermark+1. Seeding the
    // cursor here (not at 1) is what lets an existing non-empty session drain
    // its first live event instead of parking it forever waiting for
    // sequence 1.
    this.acknowledgedSequence = ack.event_watermark;
    this.nextSequence = ack.event_watermark + 1;
    return {
      ...binding,
      snapshotToken: ack.snapshot_token,
      snapshotExpiresAt: ack.expires_at || binding.expiresAt,
      resumeToken: ack.resume_token,
      consumerId: ack.consumer_id,
      watermark: ack.event_watermark,
    };
  }

  private refreshBindingIfNeeded(current: OrderedReady): Promise<OrderedReady> {
    if (new Date(current.expiresAt).getTime() - Date.now() > 2 * 60 * 1000) {
      return Promise.resolve(current);
    }
    if (this.bindingRefreshPromise) return this.bindingRefreshPromise;
    this.bindingPromise = fetchConversationBinding(this.pluginId, this.signal);
    const pending = this.bindingPromise
      .then(async (binding) => {
        if (binding.generation !== current.generation) {
          return this.rebindGeneration(current, binding);
        }
        const next = { ...current, ...binding };
        this.readyPromise = Promise.resolve(next);
        return next;
      })
      .finally(() => {
        if (this.bindingRefreshPromise === pending) this.bindingRefreshPromise = null;
      });
    this.bindingRefreshPromise = pending;
    return pending;
  }

  private project(event: RawSessionEvent, acknowledge = true): boolean {
    let projected = true;
    for (const listener of this.listeners) {
      try {
        projected = listener(event) && projected;
      } catch {
        projected = false;
      }
    }
    if (!projected) return false;
    if (event.event_type === SESSION_REMOVED_EVENT) this.terminal = true;
    if (acknowledge) {
      this.ackChain = this.ackChain
        .then(() => this.acknowledge(event.sequence))
        .catch(() => {
          this.sequenceBlocked = true;
        });
    }
    return true;
  }

  private async acknowledge(sequence: number) {
    const ready = await this.ready();
    const client = getWebSocketClient();
    if (!client || this.closed || !this.sessionId) return;
    const ack = await client.request<{ success: true; resume_token: string } | { success: false }>(
      "session.ack",
      {
        session_id: this.sessionId,
        consumer_kind: "plugin",
        consumer_id: this.consumerId,
        plugin_id: this.pluginId,
        generation: ready.generation,
        sequence,
        resume_token: this.currentResumeToken,
      },
    );
    if (!ack.success) {
      this.sequenceBlocked = true;
      return;
    }
    this.currentResumeToken = ack.resume_token;
    this.acknowledgedSequence = sequence;
  }

  private async rebindGeneration(current: OrderedReady, binding: Binding): Promise<OrderedReady> {
    return this.rebindSubscription(current, binding);
  }

  private recoverExpiredContinuation(current: OrderedReady): Promise<OrderedReady> {
    if (this.continuationRecoveryPromise) return this.continuationRecoveryPromise;
    const pending = this.rebindSubscription(current, current).finally(() => {
      if (this.continuationRecoveryPromise === pending) {
        this.continuationRecoveryPromise = null;
      }
    });
    this.continuationRecoveryPromise = pending;
    return pending;
  }

  private async rebindSubscription(current: OrderedReady, binding: Binding): Promise<OrderedReady> {
    const client = getWebSocketClient();
    if (!client || this.closed || this.terminal || !this.sessionId) {
      const next = { ...current, ...binding };
      this.readyPromise = Promise.resolve(next);
      return next;
    }
    const previousConsumerId = this.consumerId;
    this.consumerId = generateUUID();
    const ack = await client.request<OrderedSubscribeAck>(SESSION_SUBSCRIBE_ACTION, {
      session_id: this.sessionId,
      consumer_kind: "plugin",
      consumer_id: this.consumerId,
      plugin_id: this.pluginId,
      generation: binding.generation,
      ...this.managedSubscriptionIdentity(),
      binding_token: binding.bindingToken,
    });
    if (!ack.success) {
      this.consumerId = previousConsumerId;
      throw ack.error;
    }
    this.consumerId = ack.consumer_id;
    await client.request("session.unsubscribe", {
      session_id: this.sessionId,
      consumer_kind: "plugin",
      consumer_id: previousConsumerId,
      plugin_id: this.pluginId,
      generation: current.generation,
    });
    const next: OrderedReady = {
      ...binding,
      snapshotToken: ack.snapshot_token,
      snapshotExpiresAt: ack.expires_at || binding.expiresAt,
      resumeToken: ack.resume_token,
      consumerId: ack.consumer_id,
      watermark: ack.event_watermark,
    };
    this.currentResumeToken = ack.resume_token;
    this.acknowledgedSequence = ack.event_watermark;
    this.nextSequence = ack.event_watermark + 1;
    this.stateGeneration += 1;
    for (const sequence of this.pendingBySequence.keys()) {
      if (sequence <= ack.event_watermark) this.pendingBySequence.delete(sequence);
    }
    this.buffered.length = 0;
    this.committedSnapshots.clear();
    this.readyPromise = Promise.resolve(next);
    await this.notifyRebindListeners();
    this.drainPending(this.sessionId);
    return next;
  }

  private async notifyRebindListeners(): Promise<void> {
    await Promise.all(
      [...this.rebindListeners].map((listener) => Promise.resolve().then(() => listener())),
    );
  }

  private blockForPoison() {
    this.sequenceBlocked = true;
    this.poisonRebindPromise ??= this.rebindPastPoison()
      .catch(() => undefined)
      .finally(() => {
        this.poisonRebindPromise = null;
      });
  }
  private async rebindPastPoison() {
    const ready = await this.ready();
    const client = getWebSocketClient();
    if (!client) return;
    if (this.closed || this.terminal || !this.sessionId) return;
    const ack = await client.request<OrderedSubscribeAck>(SESSION_SUBSCRIBE_ACTION, {
      session_id: this.sessionId,
      consumer_kind: "plugin",
      consumer_id: this.consumerId,
      plugin_id: this.pluginId,
      generation: ready.generation,
      ...this.managedSubscriptionIdentity(),
      binding_token: ready.bindingToken,
      last_seen_sequence: this.acknowledgedSequence,
      resume_token: this.currentResumeToken,
      replace_cursor: true,
    });
    if (!ack.success) {
      if (ack.error.code === "session_removed") this.projectTerminalRemoval();
      return;
    }
    this.consumerId = ack.consumer_id;
    this.currentResumeToken = ack.resume_token;
    this.acknowledgedSequence = ack.event_watermark;
    this.nextSequence = ack.event_watermark + 1;
    for (const sequence of this.pendingBySequence.keys()) {
      if (sequence <= ack.event_watermark) this.pendingBySequence.delete(sequence);
    }
    this.buffered.length = 0;
    this.committedSnapshots.clear();
    this.stateGeneration += 1;
    this.sequenceBlocked = true;
    this.readyPromise = Promise.resolve({
      ...ready,
      snapshotToken: ack.snapshot_token,
      snapshotExpiresAt: ack.expires_at || ready.snapshotExpiresAt,
      resumeToken: ack.resume_token,
      watermark: ack.event_watermark,
    });
    await this.notifyRebindListeners();
    this.sequenceBlocked = false;
    this.drainPending(this.sessionId);
  }

  private projectTerminalRemoval() {
    if (!this.sessionId || this.terminal) return;
    this.terminal = true;
    const removedEvent: RawSessionEvent = {
      type: "session.event",
      protocol_version: 1,
      event_type: SESSION_REMOVED_EVENT,
      session_id: this.sessionId,
      task_id: this.taskId,
      sequence: this.acknowledgedSequence,
      event_id: `terminal:${this.sessionId}`,
      payload: { type: SESSION_REMOVED_EVENT, session_id: this.sessionId, task_id: this.taskId },
    };
    this.listeners.forEach((listener) => listener(removedEvent));
  }

  private async resume() {
    const ready = await this.ready();
    const client = getWebSocketClient();
    if (!client || this.closed || this.terminal || !this.sessionId) return;
    const ack = await client.request<OrderedSubscribeAck>(SESSION_SUBSCRIBE_ACTION, {
      session_id: this.sessionId,
      consumer_kind: "plugin",
      consumer_id: this.consumerId,
      plugin_id: this.pluginId,
      generation: ready.generation,
      ...this.managedSubscriptionIdentity(),
      binding_token: ready.bindingToken,
      last_seen_sequence: this.acknowledgedSequence,
      resume_token: this.currentResumeToken,
    });
    if (ack.success) {
      this.currentResumeToken = ack.resume_token;
      const invalidResume = ack.result === "invalid_resume";
      if (invalidResume) {
        this.sequenceBlocked = true;
        this.acknowledgedSequence = 0;
        this.committedSnapshots.clear();
        this.buffered.length = 0;
      }
      this.nextSequence =
        ack.replay_from ?? Math.max(ack.event_watermark + 1, this.acknowledgedSequence + 1);
      for (const sequence of this.pendingBySequence.keys()) {
        if (sequence <= ack.event_watermark) this.pendingBySequence.delete(sequence);
      }
      if (!invalidResume) this.sequenceBlocked = false;
      if (invalidResume) this.stateGeneration += 1;
      this.readyPromise = Promise.resolve({
        ...ready,
        snapshotToken: ack.snapshot_token,
        snapshotExpiresAt: ack.expires_at || ready.snapshotExpiresAt,
        resumeToken: ack.resume_token,
        watermark: ack.event_watermark,
      });
      if (invalidResume) {
        await this.notifyRebindListeners();
        this.sequenceBlocked = false;
        this.drainPending(this.sessionId);
      }
      return;
    }
    if (ack.error.code === "session_removed") this.projectTerminalRemoval();
  }
}

export function PluginConversationScopeProvider({
  pluginId,
  taskId,
  sessionId,
  generation = 0,
  managedConversationToken,
  presentation = "desktop",
  children,
}: React.PropsWithChildren<{
  pluginId: string;
  taskId: string;
  sessionId: string | null;
  generation?: number;
  managedConversationToken?: string;
  presentation?: "desktop" | "mobile";
}>) {
  const controller = React.useMemo(
    () => new AbortController(),
    [generation, managedConversationToken, pluginId, presentation, sessionId, taskId],
  );
  const sourceTransport = typeof getWebSocketClient()?.on === "function";
  const scope = React.useMemo<ConversationScope>(
    () =>
      sourceTransport
        ? new SourceConversationScope(
            pluginId,
            taskId,
            sessionId,
            controller,
            managedConversationToken,
          )
        : new OrderedConversationScope(
            pluginId,
            taskId,
            sessionId,
            controller,
            managedConversationToken,
          ),
    [controller, managedConversationToken, pluginId, sessionId, sourceTransport, taskId],
  );
  React.useLayoutEffect(() => {
    const client = getWebSocketClient();
    let sawDisconnect = false;
    const removeListener = scope.source
      ? client?.on("session.conversation.changed", (message) => {
          scope.acceptChange?.(message.payload);
        })
      : client?.onRawSessionEvent((event) => scope.accept(event));
    const removeTerminalListener = scope.source
      ? client?.on("session.removed", (message) => {
          const payload = message.payload;
          if (
            payload &&
            typeof payload === "object" &&
            "session_id" in payload &&
            payload.session_id === sessionId
          ) {
            scope.acceptTerminalRemoval?.();
          }
        })
      : undefined;
    const removeStatusListener = client?.onConnectionStatus((status) => {
      if (status !== "connected") {
        sawDisconnect = true;
        return;
      }
      if (sawDisconnect) {
        sawDisconnect = false;
        scope.reconnect();
      }
    });
    return () => {
      removeListener?.();
      removeTerminalListener?.();
      removeStatusListener?.();
      scope.close();
      controller.abort();
    };
  }, [controller, scope]);
  return (
    <ConversationScopeContext.Provider value={scope}>{children}</ConversationScopeContext.Provider>
  );
}
