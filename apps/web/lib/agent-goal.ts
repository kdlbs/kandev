import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

export type AgentGoalStatus = "active" | "paused" | "blocked" | "limited" | "complete";

export type AgentGoal = {
  objective: string;
  status: AgentGoalStatus;
  createdAt: number;
  updatedAt: number;
  tokenBudget?: number | null;
  tokensUsed?: number;
  timeUsedSeconds?: number;
  controlMethod?: string;
};

export type AgentGoalReconciliation = {
  revision: number;
  cleared: boolean;
  watermark: { createdAt: number; updatedAt: number } | null;
  /** ACP session-info timestamp of the accepted live snapshot. */
  sourceUpdatedAt?: string;
};

const GOAL_OBJECTIVE_MAX_LENGTH = 16 * 1024;
const GOAL_STATUSES = new Set<AgentGoalStatus>([
  "active",
  "paused",
  "blocked",
  "limited",
  "complete",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonNegativeFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function hasOwn(value: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function parseOptionalGoalFields(value: Record<string, unknown>, goal: AgentGoal): boolean {
  if (hasOwn(value, "tokenBudget")) {
    if (value.tokenBudget !== null && !isNonNegativeFiniteNumber(value.tokenBudget)) return false;
    goal.tokenBudget = value.tokenBudget as number | null;
  }
  for (const key of ["tokensUsed", "timeUsedSeconds"] as const) {
    if (!hasOwn(value, key)) continue;
    if (!isNonNegativeFiniteNumber(value[key])) return false;
    goal[key] = value[key];
  }
  if (hasOwn(value, "controlMethod")) {
    if (typeof value.controlMethod !== "string") return false;
    goal.controlMethod = value.controlMethod;
  }
  return true;
}

/** Parses the narrow ACP goal extension into the fields the UI is allowed to read. */
export function parseAgentGoal(value: unknown): AgentGoal | null {
  if (!isRecord(value)) return null;
  const { objective, status, createdAt, updatedAt } = value;
  if (
    typeof objective !== "string" ||
    objective.length === 0 ||
    objective.length > GOAL_OBJECTIVE_MAX_LENGTH ||
    typeof status !== "string" ||
    !GOAL_STATUSES.has(status as AgentGoalStatus) ||
    !isNonNegativeFiniteNumber(createdAt) ||
    !isNonNegativeFiniteNumber(updatedAt)
  ) {
    return null;
  }

  const goal: AgentGoal = {
    objective,
    status: status as AgentGoalStatus,
    createdAt,
    updatedAt,
  };
  return parseOptionalGoalFields(value, goal) ? goal : null;
}

/** Compares provider goal snapshots by goal identity and update time. */
export function isAgentGoalAtLeastAsFresh(incoming: AgentGoal, existing: AgentGoal): boolean {
  if (incoming.createdAt !== existing.createdAt) return incoming.createdAt > existing.createdAt;
  return incoming.updatedAt >= existing.updatedAt;
}

function isAgentGoalStrictlyNewer(
  incoming: AgentGoal,
  watermark: { createdAt: number; updatedAt: number },
): boolean {
  return (
    incoming.createdAt > watermark.createdAt ||
    (incoming.createdAt === watermark.createdAt && incoming.updatedAt > watermark.updatedAt)
  );
}

/** Returns whether an incoming session snapshot is newer than the accepted live goal snapshot. */
export function isAgentGoalSnapshotNewer(
  snapshotUpdatedAt: string | undefined,
  reconciliation: AgentGoalReconciliation | undefined,
): boolean {
  if (!snapshotUpdatedAt || !reconciliation?.sourceUpdatedAt) return false;
  const snapshotTime = parseTurnTimestamp(snapshotUpdatedAt);
  const sourceTime = parseTurnTimestamp(reconciliation.sourceUpdatedAt);
  return snapshotTime !== null && sourceTime !== null && snapshotTime > sourceTime;
}

/** Returns the accepted goal from ACP metadata, or null for absent/invalid values. */
export function getAgentGoal(metadata: unknown): AgentGoal | null {
  if (!isRecord(metadata)) return null;
  return parseAgentGoal(metadata.goal);
}

/**
 * Merges only the goal field into a sparse provider metadata replacement.
 * Other ACP metadata keeps the existing shallow replacement behavior.
 */
export function mergeAgentGoalMetadata(
  existing: Record<string, unknown> | undefined,
  incoming: Record<string, unknown> | undefined,
  options: {
    attachmentChanged?: boolean;
    source?: "live" | "hydration";
    reconciliation?: AgentGoalReconciliation;
    snapshotUpdatedAt?: string;
  } = {},
): Record<string, unknown> {
  const current = existing ?? {};
  const next = { ...(incoming ?? {}) };
  const existingHasGoal = hasOwn(current, "goal");
  const incomingHasGoal = hasOwn(incoming ?? {}, "goal");

  if (options.attachmentChanged) {
    return mergeAttachedGoal(next, incoming, incomingHasGoal);
  }
  if (!incomingHasGoal) {
    return preserveExistingGoal(next, current, existingHasGoal);
  }
  if (incoming?.goal === null) {
    return mergeClearedGoal(next, current.goal, options);
  }

  return mergeLiveGoal(next, current, incoming ?? {}, options);
}

function mergeAttachedGoal(
  next: Record<string, unknown>,
  incoming: Record<string, unknown> | undefined,
  incomingHasGoal: boolean,
): Record<string, unknown> {
  next.goal = incomingHasGoal ? parseAgentGoal(incoming?.goal) : null;
  return next;
}

function preserveExistingGoal(
  next: Record<string, unknown>,
  current: Record<string, unknown>,
  existingHasGoal: boolean,
): Record<string, unknown> {
  if (existingHasGoal) next.goal = current.goal;
  return next;
}

function mergeClearedGoal(
  next: Record<string, unknown>,
  currentGoal: unknown,
  options: {
    attachmentChanged?: boolean;
    source?: "live" | "hydration";
    reconciliation?: AgentGoalReconciliation;
    snapshotUpdatedAt?: string;
  },
): Record<string, unknown> {
  next.goal = shouldRetainHydrationGoal(currentGoal, options) ? currentGoal : null;
  return next;
}

function mergeLiveGoal(
  next: Record<string, unknown>,
  current: Record<string, unknown>,
  incoming: Record<string, unknown>,
  options: {
    attachmentChanged?: boolean;
    source?: "live" | "hydration";
    reconciliation?: AgentGoalReconciliation;
    snapshotUpdatedAt?: string;
  },
): Record<string, unknown> {
  const parsed = parseAgentGoal(incoming.goal);
  if (!parsed) {
    next.goal = null;
    return next;
  }

  const existingGoal = parseAgentGoal(current.goal);
  if (shouldRetainClearedGoal(parsed, options)) {
    next.goal = current.goal;
    return next;
  }
  next.goal =
    existingGoal && !isAgentGoalAtLeastAsFresh(parsed, existingGoal) ? existingGoal : parsed;
  return next;
}

function shouldRetainHydrationGoal(
  currentGoal: unknown,
  options: {
    source?: "live" | "hydration";
    reconciliation?: AgentGoalReconciliation;
    snapshotUpdatedAt?: string;
  },
): boolean {
  return (
    options.source === "hydration" &&
    options.reconciliation !== undefined &&
    !options.reconciliation.cleared &&
    parseAgentGoal(currentGoal) !== null &&
    !isAgentGoalSnapshotNewer(options.snapshotUpdatedAt, options.reconciliation)
  );
}

function shouldRetainClearedGoal(
  parsed: AgentGoal,
  options: { source?: "live" | "hydration"; reconciliation?: AgentGoalReconciliation },
): boolean {
  if (!options.reconciliation?.cleared) return false;
  const watermark = options.reconciliation.watermark;
  return (
    (!watermark && options.source !== "live") ||
    (watermark !== null && !isAgentGoalStrictlyNewer(parsed, watermark))
  );
}
