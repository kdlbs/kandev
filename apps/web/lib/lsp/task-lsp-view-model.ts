import type { TaskLspLanguageSnapshot, TaskLspPhase } from "@/lib/types/http-lsp";

export type TaskLspVisualState =
  | "error"
  | "server_work"
  | "initializing"
  | "installing"
  | "starting"
  | "queued"
  | "stopping"
  | "ready"
  | "unsupported"
  | "waiting"
  | "detected"
  | "configured"
  | "stopped"
  | "off";

export type TaskLspLanguageView = {
  language: string;
  label: string;
  state: TaskLspVisualState;
  relevant: boolean;
  elapsedMs: number | null;
  work: TaskLspLanguageSnapshot["progress"][number] | null;
  actions: { start: boolean; stop: boolean; restart: boolean };
  snapshot: TaskLspLanguageSnapshot;
};

export type TaskLspCompactSummary =
  | { kind: "empty" }
  | {
      kind: "single";
      language: string;
      state: TaskLspVisualState;
      workTitle?: string;
      elapsedMs: number | null;
    }
  | {
      kind: "multiple";
      runningCount: number;
      errorCount: number;
      workCount: number;
      relevantCount: number;
    };

export type TaskLspViewModel = {
  rows: TaskLspLanguageView[];
  relevantRows: TaskLspLanguageView[];
  hiddenCount: number;
  aggregate: {
    compact: TaskLspCompactSummary;
    runningCount: number;
    errorCount: number;
    workCount: number;
  };
};

export type TaskLspViewOptions = {
  hiddenLanguages?: readonly string[];
  forceVisibleLanguage?: string | null;
};

const LABELS: Record<string, string> = {
  go: "Go",
  kotlin: "Kotlin",
  python: "Python",
  rust: "Rust",
  typescript: "TypeScript / JavaScript",
};

const PRIORITY: Record<TaskLspVisualState, number> = {
  error: 0,
  server_work: 1,
  initializing: 2,
  installing: 3,
  starting: 4,
  queued: 5,
  stopping: 6,
  ready: 7,
  unsupported: 8,
  waiting: 9,
  detected: 10,
  configured: 11,
  stopped: 12,
  off: 13,
};

const PHASE_VISUAL_STATE: Partial<Record<TaskLspPhase, TaskLspVisualState>> = {
  error: "error",
  unsupported: "unsupported",
  initializing: "initializing",
  process_started: "initializing",
  installing: "installing",
  starting: "starting",
  queued: "queued",
  stopping: "stopping",
  ready: "ready",
  waiting_for_task: "waiting",
};

export function taskLspLanguageLabel(language: string): string {
  return LABELS[language] ?? language.charAt(0).toUpperCase() + language.slice(1);
}

function visualState(snapshot: TaskLspLanguageSnapshot): TaskLspVisualState {
  if (snapshot.progress.length > 0 || snapshot.activity === "server_work") return "server_work";
  const activeState = PHASE_VISUAL_STATE[snapshot.phase];
  if (activeState) return activeState;
  if (snapshot.policy === "disabled") return "stopped";
  if (snapshot.detected) return "detected";
  if (snapshot.policy === "keep_warm" || snapshot.effective_policy === "keep_warm") {
    return "configured";
  }
  return "off";
}

function isRelevant(snapshot: TaskLspLanguageSnapshot, state: TaskLspVisualState): boolean {
  return (
    snapshot.detected ||
    snapshot.policy !== "inherit" ||
    snapshot.effective_policy === "keep_warm" ||
    snapshot.restart_required ||
    snapshot.detection_state === "scanning" ||
    snapshot.detection_state === "partial" ||
    snapshot.detection_state === "unavailable" ||
    state !== "off"
  );
}

function phaseHasServer(phase: TaskLspPhase): boolean {
  return [
    "installing",
    "starting",
    "process_started",
    "initializing",
    "ready",
    "stopping",
  ].includes(phase);
}

function actions(snapshot: TaskLspLanguageSnapshot) {
  if (snapshot.phase === "unsupported" || snapshot.phase === "stopping") {
    return { start: false, stop: false, restart: false };
  }
  const hasServer = phaseHasServer(snapshot.phase);
  const restart = snapshot.generation > 0 && (hasServer || snapshot.phase === "error");
  return {
    start: !hasServer && snapshot.phase !== "queued" && !restart,
    stop:
      hasServer ||
      snapshot.phase === "queued" ||
      snapshot.phase === "error" ||
      snapshot.detected ||
      snapshot.policy !== "disabled",
    restart,
  };
}

function parseTime(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function elapsedStart(snapshot: TaskLspLanguageSnapshot): number | null {
  return (
    parseTime(snapshot.progress[0]?.started_at) ??
    parseTime(snapshot.initialize_started_at) ??
    parseTime(snapshot.process_started_at)
  );
}

function row(snapshot: TaskLspLanguageSnapshot, now: number): TaskLspLanguageView {
  const state = visualState(snapshot);
  const startedAt = elapsedStart(snapshot);
  return {
    language: snapshot.language,
    label: taskLspLanguageLabel(snapshot.language),
    state,
    relevant: isRelevant(snapshot, state),
    elapsedMs: startedAt === null ? null : Math.max(0, now - startedAt),
    work: snapshot.progress[0] ?? null,
    actions: actions(snapshot),
    snapshot,
  };
}

function compactSummary(
  relevantRows: TaskLspLanguageView[],
  runningCount: number,
  errorCount: number,
  workCount: number,
): TaskLspCompactSummary {
  if (relevantRows.length === 0) return { kind: "empty" };
  if (relevantRows.length > 1) {
    return {
      kind: "multiple",
      runningCount,
      errorCount,
      workCount,
      relevantCount: relevantRows.length,
    };
  }
  const only = relevantRows[0];
  return {
    kind: "single",
    language: only.label,
    state: only.state,
    ...(only.work ? { workTitle: only.work.title } : {}),
    elapsedMs: only.elapsedMs,
  };
}

export function deriveTaskLspViewModel(
  languages: readonly TaskLspLanguageSnapshot[],
  now: number,
  options: TaskLspViewOptions = {},
): TaskLspViewModel {
  const hiddenLanguages = new Set(options.hiddenLanguages ?? []);
  const allRows = languages
    .map((snapshot) => row(snapshot, now))
    .sort((a, b) => a.label.localeCompare(b.label));
  const hiddenCount = allRows.filter((candidate) => hiddenLanguages.has(candidate.language)).length;
  const rows = allRows.filter(
    (candidate) =>
      !hiddenLanguages.has(candidate.language) ||
      candidate.language === options.forceVisibleLanguage,
  );
  const relevantRows = rows
    .filter((candidate) => candidate.relevant)
    .sort((a, b) => PRIORITY[a.state] - PRIORITY[b.state] || a.label.localeCompare(b.label));
  const runningCount = relevantRows.filter((candidate) =>
    phaseHasServer(candidate.snapshot.phase),
  ).length;
  const errorCount = relevantRows.filter((candidate) => candidate.state === "error").length;
  const workCount = relevantRows.filter((candidate) => candidate.state === "server_work").length;
  return {
    rows,
    relevantRows,
    hiddenCount,
    aggregate: {
      compact: compactSummary(relevantRows, runningCount, errorCount, workCount),
      runningCount,
      errorCount,
      workCount,
    },
  };
}
