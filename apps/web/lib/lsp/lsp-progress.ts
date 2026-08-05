export type LspProgressToken = string | number;

export type LspProgressWorkItem = {
  token: LspProgressToken;
  title: string;
  message: string | null;
  percentage: number | null;
  startedAt: number;
};

export type LspCompletedWorkItem = {
  token: LspProgressToken;
  title: string;
  message: string | null;
  startedAt: number;
  completedAt: number;
};

export type LspProgressSnapshot = {
  initializingSince: number | null;
  active: readonly LspProgressWorkItem[];
  completed: LspCompletedWorkItem | null;
  hasReportedProgress: boolean;
};

export type LspProgressParams = {
  token?: unknown;
  value?: unknown;
};

export const EMPTY_LSP_PROGRESS: LspProgressSnapshot = {
  initializingSince: null,
  active: [],
  completed: null,
  hasReportedProgress: false,
};

export function createLspProgressSnapshot(initializingSince: number | null): LspProgressSnapshot {
  return {
    ...EMPTY_LSP_PROGRESS,
    initializingSince,
  };
}

export function finishLspInitialization(snapshot: LspProgressSnapshot): LspProgressSnapshot {
  if (snapshot.initializingSince === null) return snapshot;
  return { ...snapshot, initializingSince: null };
}

export function isLspProgressToken(value: unknown): value is LspProgressToken {
  return typeof value === "string" || (typeof value === "number" && Number.isFinite(value));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasValidOptionalString(value: Record<string, unknown>, key: string): boolean {
  return !(key in value) || typeof value[key] === "string";
}

function hasValidOptionalPercentage(value: Record<string, unknown>): boolean {
  return (
    !("percentage" in value) ||
    (typeof value.percentage === "number" && Number.isFinite(value.percentage))
  );
}

function clampPercentage(percentage: number): number {
  return Math.min(100, Math.max(0, percentage));
}

function applyBegin(
  snapshot: LspProgressSnapshot,
  token: LspProgressToken,
  value: Record<string, unknown>,
  receivedAt: number,
): LspProgressSnapshot {
  if (
    typeof value.title !== "string" ||
    !hasValidOptionalString(value, "message") ||
    !hasValidOptionalPercentage(value)
  ) {
    return snapshot;
  }

  const item: LspProgressWorkItem = {
    token,
    title: value.title,
    message: typeof value.message === "string" ? value.message : null,
    percentage: typeof value.percentage === "number" ? clampPercentage(value.percentage) : null,
    startedAt: receivedAt,
  };

  return {
    ...snapshot,
    active: [...snapshot.active.filter((active) => active.token !== token), item],
    hasReportedProgress: true,
  };
}

function applyReport(
  snapshot: LspProgressSnapshot,
  token: LspProgressToken,
  value: Record<string, unknown>,
): LspProgressSnapshot {
  if (!hasValidOptionalString(value, "message") || !hasValidOptionalPercentage(value)) {
    return snapshot;
  }
  const index = snapshot.active.findIndex((active) => active.token === token);
  if (index < 0) return snapshot;
  if (!("message" in value) && !("percentage" in value)) return snapshot;

  const current = snapshot.active[index];
  const next: LspProgressWorkItem = {
    ...current,
    message: typeof value.message === "string" ? value.message : current.message,
    percentage:
      typeof value.percentage === "number" ? clampPercentage(value.percentage) : current.percentage,
  };
  if (next.message === current.message && next.percentage === current.percentage) return snapshot;

  const active = [...snapshot.active];
  active[index] = next;
  return { ...snapshot, active, hasReportedProgress: true };
}

function applyEnd(
  snapshot: LspProgressSnapshot,
  token: LspProgressToken,
  value: Record<string, unknown>,
  receivedAt: number,
): LspProgressSnapshot {
  if (!hasValidOptionalString(value, "message")) return snapshot;
  const item = snapshot.active.find((active) => active.token === token);
  if (!item) return snapshot;

  return {
    ...snapshot,
    active: snapshot.active.filter((active) => active.token !== token),
    completed: {
      token,
      title: item.title,
      message: typeof value.message === "string" ? value.message : null,
      startedAt: item.startedAt,
      completedAt: receivedAt,
    },
    hasReportedProgress: true,
  };
}

export function applyLspProgress(
  snapshot: LspProgressSnapshot,
  registeredTokens: ReadonlySet<LspProgressToken>,
  params: unknown,
  receivedAt: number,
): LspProgressSnapshot {
  if (!isRecord(params) || !isLspProgressToken(params.token)) return snapshot;
  if (!registeredTokens.has(params.token) || !isRecord(params.value)) return snapshot;

  switch (params.value.kind) {
    case "begin":
      return applyBegin(snapshot, params.token, params.value, receivedAt);
    case "report":
      return applyReport(snapshot, params.token, params.value);
    case "end":
      return applyEnd(snapshot, params.token, params.value, receivedAt);
    default:
      return snapshot;
  }
}
