import { createStore } from "zustand/vanilla";
import {
  contextContinuationDetails,
  type ContextContinuationDetails,
} from "@/lib/services/session-recovery-service";
import type { SessionRecoveryBlockProjection, TaskSession } from "@/lib/types/http";
import {
  readAgentDeliveryRecovery,
  type AgentDeliveryRecovery,
} from "@/lib/session-agent-delivery-recovery";

export type SessionRecoveryGuidanceIdentity = {
  taskId: string;
  sessionId: string;
  requestKey: string;
  errorStamp?: string | null;
  sessionState?: string;
  generation?: number;
  blockId?: string;
  incarnationId?: string;
  reason?: string;
  deliveryPhase?: AgentDeliveryRecovery["phase"];
  deliveryRevision?: number;
  deliverySubmissionId?: string;
  openDeliveryBlockIds?: string[];
};

export type SessionRecoveryGuidanceEntry = {
  details: ContextContinuationDetails;
  identity: SessionRecoveryGuidanceIdentity;
  operationId: number;
  observedBlockId?: string;
};

export type SessionRecoveryGuidanceOperation = {
  key: string;
  operationId: number;
  identity: SessionRecoveryGuidanceIdentity;
};

export type SessionRecoveryGuidanceState = {
  entries: ReadonlyMap<string, SessionRecoveryGuidanceEntry>;
  operations: ReadonlyMap<string, number>;
};

type GuidanceStore = ReturnType<typeof createGuidanceStore>;
const storesByOwner = new WeakMap<object, GuidanceStore>();

export function deliveryRecoveryGuidanceIdentity(
  recovery: AgentDeliveryRecovery | null,
  block: SessionRecoveryBlockProjection | undefined,
): string | null {
  if (recovery) {
    return JSON.stringify([
      recovery.submissionId,
      recovery.streamId,
      recovery.incarnationId,
      recovery.harnessGeneration,
      recovery.promptGeneration,
      block?.id,
      block?.incarnation_id,
      block?.expected_generation,
      block?.reason,
    ]);
  }
  if (!block) return null;
  return `block:${block.id}:${block.incarnation_id}:${block.expected_generation}:${block.reason}`;
}

export function sessionRecoveryGuidanceIdentityFromSession(
  taskId: string,
  session: Pick<TaskSession, "id" | "state" | "metadata" | "session_recovery_blocks">,
  errorStamp?: string | null,
): SessionRecoveryGuidanceIdentity {
  const projection = sessionDeliveryProjection(session);
  return {
    taskId,
    sessionId: session.id,
    requestKey: sessionRecoveryRequestKey(taskId, session.id, errorStamp, projection.identity),
    errorStamp,
    sessionState: session.state,
    ...sessionDeliveryIdentityFields(projection),
  };
}

function sessionRecoveryRequestKey(
  taskId: string,
  sessionId: string,
  errorStamp: string | null | undefined,
  deliveryIdentity: string | null,
) {
  const identity = deliveryIdentity ?? errorStamp ?? "";
  return `${taskId}\u0000${sessionId}\u0000${identity}`;
}

function sessionDeliveryIdentityFields(
  projection: ReturnType<typeof sessionDeliveryProjection>,
): Pick<
  SessionRecoveryGuidanceIdentity,
  | "generation"
  | "blockId"
  | "incarnationId"
  | "reason"
  | "deliveryPhase"
  | "deliveryRevision"
  | "deliverySubmissionId"
  | "openDeliveryBlockIds"
> {
  const fields: Pick<
    SessionRecoveryGuidanceIdentity,
    | "generation"
    | "blockId"
    | "incarnationId"
    | "reason"
    | "deliveryPhase"
    | "deliveryRevision"
    | "deliverySubmissionId"
    | "openDeliveryBlockIds"
  > = {};
  if (projection.block) {
    fields.generation = projection.block.expected_generation;
    fields.blockId = projection.block.id;
    fields.incarnationId = projection.block.incarnation_id;
    fields.reason = projection.block.reason;
    fields.deliverySubmissionId =
      projection.block.delivery_submission_id ?? projection.recovery?.submissionId;
  } else if (projection.recovery) {
    fields.generation = projection.recovery.harnessGeneration;
    fields.incarnationId = projection.recovery.incarnationId;
    fields.deliverySubmissionId = projection.recovery.submissionId;
  }
  if (projection.recovery) {
    fields.deliveryPhase = projection.recovery.phase;
    fields.deliveryRevision = projection.recovery.revision;
  }
  if (projection.openBlockIds !== undefined) {
    fields.openDeliveryBlockIds = projection.openBlockIds;
  }
  return fields;
}

function sessionDeliveryProjection(
  session: Pick<TaskSession, "metadata" | "session_recovery_blocks">,
) {
  const recovery = readAgentDeliveryRecovery(session.metadata);
  const blocks = session.session_recovery_blocks;
  const block = blocks?.find((candidate) => candidate.consumer_reference === "agent_delivery");
  return {
    recovery,
    block,
    identity: deliveryRecoveryGuidanceIdentity(recovery, block),
    openBlockIds: blocks?.flatMap((candidate) =>
      candidate.consumer_reference === "agent_delivery" ? [candidate.id] : [],
    ),
  };
}

function createGuidanceStore() {
  return createStore<SessionRecoveryGuidanceState>(() => ({
    entries: new Map(),
    operations: new Map(),
  }));
}

/** Keeps transient continuation guidance inside the owning application store. */
export function getSessionRecoveryGuidanceStore(owner: object) {
  let store = storesByOwner.get(owner);
  if (!store) {
    store = createGuidanceStore();
    storesByOwner.set(owner, store);
  }
  return store;
}

export function sessionRecoveryGuidanceKey(taskId: string, sessionId: string) {
  return `${taskId}\u0000${sessionId}`;
}

/** Starts an ordered request whose late result cannot replace newer guidance. */
export function beginSessionRecoveryGuidanceOperation(
  owner: object,
  identity: SessionRecoveryGuidanceIdentity,
): SessionRecoveryGuidanceOperation {
  const store = getSessionRecoveryGuidanceStore(owner);
  const key = sessionRecoveryGuidanceKey(identity.taskId, identity.sessionId);
  const state = store.getState();
  const operationId = (state.operations.get(key) ?? 0) + 1;
  const operations = new Map(state.operations);
  operations.set(key, operationId);
  store.setState({ ...state, operations });
  return { key, operationId, identity: { ...identity } };
}

/** Publishes only a structured hint that still belongs to this request and incident. */
export function publishSessionRecoveryGuidance(
  owner: object,
  operation: SessionRecoveryGuidanceOperation,
  error: unknown,
  currentIdentity: SessionRecoveryGuidanceIdentity = operation.identity,
): boolean {
  const store = getSessionRecoveryGuidanceStore(owner);
  const state = store.getState();
  if (state.operations.get(operation.key) !== operation.operationId) return false;
  if (!sameTarget(operation.identity, currentIdentity)) return false;

  const details = contextContinuationDetails(error, {
    sessionId: currentIdentity.sessionId,
    generation: currentIdentity.generation,
    blockId: currentIdentity.blockId,
    incarnationId: currentIdentity.incarnationId,
  });
  if (!details || !matchesCapturedIncident(operation.identity, currentIdentity, details))
    return false;

  const identity = mergeIdentity(operation.identity, currentIdentity, details);
  const capturedBlockId = identity.blockId;
  const observedBlockId =
    capturedBlockId &&
    [operation.identity.openDeliveryBlockIds, currentIdentity.openDeliveryBlockIds].some(
      (blocks) => blocks?.includes(capturedBlockId) === true,
    )
      ? capturedBlockId
      : undefined;
  const entry = { details, identity, operationId: operation.operationId, observedBlockId };
  if (guidanceResolvedByProjection(entry, currentIdentity, operation.identity)) {
    invalidateSessionRecoveryGuidance(owner, identity.taskId, identity.sessionId);
    return false;
  }
  const entries = new Map(state.entries);
  entries.set(operation.key, entry);
  store.setState({ ...state, entries });
  return true;
}

export function selectSessionRecoveryGuidance(
  owner: object,
  currentIdentity: SessionRecoveryGuidanceIdentity,
): SessionRecoveryGuidanceEntry | null {
  const store = getSessionRecoveryGuidanceStore(owner);
  const key = sessionRecoveryGuidanceKey(currentIdentity.taskId, currentIdentity.sessionId);
  const entry = store.getState().entries.get(key);
  if (!entry || !isSessionRecoveryGuidanceCurrent(entry, currentIdentity)) return null;
  return entry;
}

export function isSessionRecoveryGuidanceCurrent(
  entry: SessionRecoveryGuidanceEntry,
  currentIdentity: SessionRecoveryGuidanceIdentity,
) {
  return (
    matchesGuidanceIdentity(entry.identity, currentIdentity) &&
    !guidanceResolvedByProjection(entry, currentIdentity)
  );
}

/** Records canonical observation and retires guidance after matching recovery evidence. */
export function reconcileSessionRecoveryGuidanceProjection(
  owner: object,
  currentIdentity: SessionRecoveryGuidanceIdentity,
) {
  const store = getSessionRecoveryGuidanceStore(owner);
  const key = sessionRecoveryGuidanceKey(currentIdentity.taskId, currentIdentity.sessionId);
  const state = store.getState();
  const entry = state.entries.get(key);
  if (!entry) return;
  if (!isSessionRecoveryGuidanceCurrent(entry, currentIdentity)) {
    invalidateSessionRecoveryGuidance(owner, currentIdentity.taskId, currentIdentity.sessionId);
    return;
  }
  const observedBlockId = entry.identity.blockId;
  if (
    !entry.observedBlockId &&
    observedBlockId &&
    currentIdentity.openDeliveryBlockIds?.includes(observedBlockId)
  ) {
    const entries = new Map(state.entries);
    entries.set(key, { ...entry, observedBlockId });
    store.setState({ ...state, entries });
  }
}

/** Confirms that a response for an older request names the projected incident. */
export function matchesContinuationDetailsToIdentity(
  details: ContextContinuationDetails | null,
  identity: SessionRecoveryGuidanceIdentity,
): boolean {
  if (!details || (details.session_id && details.session_id !== identity.sessionId)) return false;
  const expected = [
    [identity.blockId, details.block_id],
    [identity.incarnationId, details.incarnation_id],
    [identity.generation, details.generation],
  ] as const;
  const known = expected.filter(([value]) => value !== undefined);
  return (
    known.length > 0 &&
    known.every(([value, actual]) => value === actual) &&
    (!identity.reason || !details.reason || identity.reason === details.reason)
  );
}

/** Clears guidance only when the successful operation is still the latest for this session. */
export function clearSessionRecoveryGuidance(
  owner: object,
  operation: SessionRecoveryGuidanceOperation,
  currentIdentity: SessionRecoveryGuidanceIdentity = operation.identity,
): boolean {
  const store = getSessionRecoveryGuidanceStore(owner);
  const state = store.getState();
  if (state.operations.get(operation.key) !== operation.operationId) return false;
  const entry = state.entries.get(operation.key);
  if (!entry || !matchesGuidanceIdentity(entry.identity, currentIdentity)) return false;
  const entries = new Map(state.entries);
  entries.delete(operation.key);
  store.setState({ ...state, entries });
  return true;
}

/** Removes guidance and fences every request that was started for a deleted session. */
export function invalidateSessionRecoveryGuidance(
  owner: object,
  taskId: string,
  sessionId: string,
): boolean {
  const store = getSessionRecoveryGuidanceStore(owner);
  const state = store.getState();
  const key = sessionRecoveryGuidanceKey(taskId, sessionId);
  const entries = new Map(state.entries);
  const existed = entries.delete(key);
  const operations = new Map(state.operations);
  operations.set(key, (operations.get(key) ?? 0) + 1);
  store.setState({ ...state, entries, operations });
  return existed;
}

export function matchesGuidanceIdentity(
  stored: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
): boolean {
  if (!sameTarget(stored, current)) return false;
  if (stored.errorStamp && current.errorStamp && stored.errorStamp !== current.errorStamp) {
    if (!sameBlock(stored, current)) return false;
  }
  if (!matchesKnownIncidentFields(stored, current)) return false;
  const hasCanonicalIdentity = hasIncidentIdentity(stored);
  if (hasCanonicalIdentity) return true;
  if (stored.errorStamp && current.errorStamp) return stored.errorStamp === current.errorStamp;
  return stored.requestKey === current.requestKey;
}

function matchesKnownIncidentFields(
  stored: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  for (const field of ["blockId", "incarnationId", "generation", "reason"] as const) {
    const storedValue = stored[field];
    const currentValue = current[field];
    if (
      (storedValue !== undefined && currentValue !== undefined && storedValue !== currentValue) ||
      (currentValue !== undefined && storedValue === undefined)
    ) {
      return false;
    }
  }
  return true;
}

function guidanceResolvedByProjection(
  entry: SessionRecoveryGuidanceEntry,
  current: SessionRecoveryGuidanceIdentity,
  baseline: SessionRecoveryGuidanceIdentity = entry.identity,
) {
  if (isAuthoritativelyUsableSession(entry.identity, current)) return true;
  const stored = entry.identity;
  if (!sameIncidentGeneration(stored, current)) return false;
  return (
    isResolvedDeliveryProjection(baseline, current) || hasObservedBlockBeenResolved(entry, current)
  );
}

function isAuthoritativelyUsableSession(
  stored: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  return stored.sessionState !== "RUNNING" && current.sessionState === "RUNNING";
}

function sameIncidentGeneration(
  stored: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  return Boolean(
    stored.incarnationId !== undefined &&
    stored.incarnationId === current.incarnationId &&
    stored.generation !== undefined &&
    stored.generation === current.generation,
  );
}

function isResolvedDeliveryProjection(
  stored: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  const deliveryBlocks = current.openDeliveryBlockIds;
  const storedBlockRemainsOpen =
    stored.blockId !== undefined && deliveryBlocks?.includes(stored.blockId) === true;
  const matchingSubmission =
    stored.deliverySubmissionId !== undefined &&
    stored.deliverySubmissionId === current.deliverySubmissionId;
  const newerProjection =
    stored.deliveryRevision !== undefined &&
    current.deliveryRevision !== undefined &&
    current.deliveryRevision > stored.deliveryRevision;
  if (
    isResolvedDeliveryPhase(current.deliveryPhase) &&
    !storedBlockRemainsOpen &&
    matchingSubmission &&
    newerProjection
  ) {
    return true;
  }
  return false;
}

function hasObservedBlockBeenResolved(
  entry: SessionRecoveryGuidanceEntry,
  current: SessionRecoveryGuidanceIdentity,
) {
  const deliveryBlocks = current.openDeliveryBlockIds;
  return (
    entry.observedBlockId !== undefined &&
    deliveryBlocks !== undefined &&
    !deliveryBlocks.includes(entry.observedBlockId)
  );
}

function isResolvedDeliveryPhase(phase: AgentDeliveryRecovery["phase"] | undefined) {
  return (
    phase === "recovered" || phase === "settled" || phase === "continued" || phase === "restored"
  );
}

function hasIncidentIdentity(identity: SessionRecoveryGuidanceIdentity) {
  return Boolean(identity.blockId || identity.incarnationId || identity.generation !== undefined);
}

function matchesCapturedIncident(
  captured: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
  details: ContextContinuationDetails,
): boolean {
  if (!matchesCapturedRequest(captured, current)) return false;
  return matchesCapturedIncidentFields(captured, current, details);
}

function matchesCapturedRequest(
  captured: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  if (!sameTarget(captured, current)) return false;
  if (captured.errorStamp && current.errorStamp && captured.errorStamp !== current.errorStamp) {
    if (!sameBlock(captured, current)) return false;
  }
  return matchesCapturedDeliverySubmission(captured, current);
}

function matchesCapturedDeliverySubmission(
  captured: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
) {
  return !(
    captured.deliverySubmissionId &&
    current.deliverySubmissionId &&
    captured.deliverySubmissionId !== current.deliverySubmissionId
  );
}

function matchesCapturedIncidentFields(
  captured: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
  details: ContextContinuationDetails,
) {
  for (const field of ["blockId", "incarnationId", "generation", "reason"] as const) {
    const capturedValue = captured[field];
    const currentValue = current[field];
    if (capturedValue !== undefined && currentValue !== undefined && capturedValue !== currentValue)
      return false;
    if (
      currentValue !== undefined &&
      capturedValue === undefined &&
      detailIdentityValue(details, field) !== currentValue
    ) {
      return false;
    }
  }
  return true;
}

function mergeIdentity(
  captured: SessionRecoveryGuidanceIdentity,
  current: SessionRecoveryGuidanceIdentity,
  details: ContextContinuationDetails,
): SessionRecoveryGuidanceIdentity {
  return {
    ...current,
    ...captured,
    requestKey: captured.requestKey,
    errorStamp: captured.errorStamp ?? current.errorStamp,
    blockId: captured.blockId ?? current.blockId ?? details.block_id,
    incarnationId: captured.incarnationId ?? current.incarnationId ?? details.incarnation_id,
    generation: captured.generation ?? current.generation ?? details.generation,
    reason: captured.reason ?? current.reason ?? details.reason,
    deliveryPhase: current.deliveryPhase ?? captured.deliveryPhase,
    deliveryRevision: current.deliveryRevision ?? captured.deliveryRevision,
    deliverySubmissionId: captured.deliverySubmissionId ?? current.deliverySubmissionId,
    openDeliveryBlockIds: current.openDeliveryBlockIds ?? captured.openDeliveryBlockIds,
    sessionState: captured.sessionState ?? current.sessionState,
  };
}

function detailIdentityValue(
  details: ContextContinuationDetails,
  field: "blockId" | "incarnationId" | "generation" | "reason",
) {
  if (field === "blockId") return details.block_id;
  if (field === "incarnationId") return details.incarnation_id;
  return details[field === "generation" ? "generation" : "reason"];
}

function sameTarget(left: SessionRecoveryGuidanceIdentity, right: SessionRecoveryGuidanceIdentity) {
  return left.taskId === right.taskId && left.sessionId === right.sessionId;
}

function sameBlock(left: SessionRecoveryGuidanceIdentity, right: SessionRecoveryGuidanceIdentity) {
  return Boolean(
    left.blockId &&
    right.blockId &&
    left.blockId === right.blockId &&
    left.incarnationId === right.incarnationId &&
    left.generation === right.generation,
  );
}
