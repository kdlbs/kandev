import type {
  ControlAction,
  EligibilitySettings,
  CoordinatorSettings,
  ProjectEntry,
  PolicyActions,
  ProjectsRequest,
  PutSettingsRequest,
  SettingsAction,
} from "@/lib/api/domains/coordinator-api";

export const CONTROL_ACTIONS: readonly ControlAction[] = [
  "create_task",
  "start_agent",
  "message",
  "move",
  "resume",
  "stop",
];

/** The classes the settings page edits: the six control classes plus `answer` when the server names it. */
export const SETTINGS_ACTIONS: readonly SettingsAction[] = [...CONTROL_ACTIONS, "answer"];

/** i18n keys of the display label of each action; the wire token is never shown. */
export const ACTION_LABEL_KEY: Record<SettingsAction, string> = {
  create_task: "coordinator:mayDoCreateTask",
  start_agent: "coordinator:mayDoStartAgent",
  message: "coordinator:mayDoMessage",
  move: "coordinator:mayDoMove",
  resume: "coordinator:mayDoResume",
  stop: "coordinator:mayDoStop",
  answer: "coordinator:mayDoAnswer",
};

export const MAX_WATCHED_BOARDS = 50;

export type WatchesDraft = { scope: "all" | "selected"; workflowIds: string[] };

export const MAX_WATCHED_PROJECTS = 50;

export type ProjectsDraft = {
  scope: "all" | "selected";
  entries: ProjectEntry[];
  includeNoRepository: boolean;
};

export const ELIGIBILITY_FIELDS = [
  "evidence_window_hours",
  "min_decisions",
  "unedited_floor_percent",
  "review_max_age_hours",
  "daily_limit",
] as const;

export type EligibilityField = (typeof ELIGIBILITY_FIELDS)[number];

/** The server's bounds (internal/coordinator/policy.go), enforced client-side before a save. */
export const ELIGIBILITY_BOUNDS: Record<EligibilityField, { min: number; max: number }> = {
  evidence_window_hours: { min: 24, max: 2160 },
  min_decisions: { min: 3, max: 500 },
  unedited_floor_percent: { min: 50, max: 100 },
  review_max_age_hours: { min: 1, max: 720 },
  daily_limit: { min: 1, max: 100 },
};

export function isEligibilityValueValid(field: EligibilityField, value: number): boolean {
  const { min, max } = ELIGIBILITY_BOUNDS[field];
  return Number.isInteger(value) && value >= min && value <= max;
}

export function sameEligibility(
  a: EligibilitySettings | null,
  b: EligibilitySettings | null,
): boolean {
  if (a === null || b === null) return a === b;
  return ELIGIBILITY_FIELDS.every((field) => a[field] === b[field]);
}

export type ControlDraft = {
  actions: PolicyActions;
  /** Null while phase 3 is not effective: the settings read carries no `policy.eligibility`. */
  eligibility: EligibilitySettings | null;
  watches: WatchesDraft;
  /** Null while the Projects scope is not offered (the settings read carries no `projects_config`). */
  projects: ProjectsDraft | null;
};

export function draftFromSettings(settings: CoordinatorSettings): ControlDraft {
  const config = settings.projects_config;
  return {
    actions: { ...settings.policy.actions },
    eligibility: settings.policy.eligibility ? { ...settings.policy.eligibility } : null,
    watches: { scope: settings.watches.scope, workflowIds: [...settings.watches.workflow_ids] },
    projects: config
      ? {
          scope: config.scope,
          entries: config.entries.map((entry) => ({ ...entry })),
          includeNoRepository: config.include_no_repository,
        }
      : null,
  };
}

const entryKey = (entry: ProjectEntry) => `${entry.kind}:${entry.id}`;

/** The server's equality: under `all` only the scope counts; selected compares entries as a set and the toggle. */
export function sameProjects(a: ProjectsDraft | null, b: ProjectsDraft | null): boolean {
  if (a === null || b === null) return a === b;
  if (a.scope !== b.scope) return false;
  if (a.scope === "all") return true;
  if (a.includeNoRepository !== b.includeNoRepository) return false;
  if (a.entries.length !== b.entries.length) return false;
  const keys = new Set(b.entries.map(entryKey));
  return a.entries.every((entry) => keys.has(entryKey(entry)));
}

export function isProjectsDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return !sameProjects(draft.projects, stored.projects);
}

/** A selected draft that watches nothing: no entry and the no-repository toggle off. It cannot be saved. */
export function isProjectsInvalid(draft: ControlDraft, stored: ControlDraft): boolean {
  const projects = draft.projects;
  return (
    projects !== null &&
    projects.scope === "selected" &&
    projects.entries.length === 0 &&
    !projects.includeNoRepository &&
    isProjectsDirty(draft, stored)
  );
}

export function projectsRequest(projects: ProjectsDraft): ProjectsRequest {
  if (projects.scope === "all") return { scope: "all" };
  return {
    scope: "selected",
    entries: projects.entries.map((entry) => ({ ...entry })),
    include_no_repository: projects.includeNoRepository,
  };
}

/** A project row of the editor, as the workspace lists it. */
export type ProjectChoice = ProjectEntry & { name: string };

/**
 * Turning "watch every project" off restores the stored entries, or starts from
 * every set and every repository outside a set, at most 50, when there are none.
 */
export function switchOffProjects(
  current: ProjectsDraft,
  choices: readonly ProjectChoice[],
): ProjectsDraft | null {
  if (current.entries.length > 0 || current.includeNoRepository) {
    return { ...current, scope: "selected" };
  }
  if (choices.length === 0) return null;
  const entries = choices.slice(0, MAX_WATCHED_PROJECTS).map(({ kind, id }) => ({ kind, id }));
  return { scope: "selected", entries, includeNoRepository: false };
}

/** Both the stored and the draft `all` scope ignore ids; selected sets compare as sets. */
export function sameWatches(a: WatchesDraft, b: WatchesDraft): boolean {
  if (a.scope !== b.scope) return false;
  if (a.scope === "all") return true;
  if (a.workflowIds.length !== b.workflowIds.length) return false;
  const ids = new Set(b.workflowIds);
  return a.workflowIds.every((id) => ids.has(id));
}

export function isEligibilityDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return !sameEligibility(draft.eligibility, stored.eligibility);
}

/** A draft eligibility with a value outside its bounds that the manager edited: it cannot be saved. */
export function isEligibilityInvalid(draft: ControlDraft, stored: ControlDraft): boolean {
  const eligibility = draft.eligibility;
  return (
    eligibility !== null &&
    isEligibilityDirty(draft, stored) &&
    ELIGIBILITY_FIELDS.some((field) => !isEligibilityValueValid(field, eligibility[field]))
  );
}

export function isPolicyDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return (
    SETTINGS_ACTIONS.some((action) => draft.actions[action] !== stored.actions[action]) ||
    isEligibilityDirty(draft, stored)
  );
}

export function isWatchesDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return !sameWatches(draft.watches, stored.watches);
}

/** A selected draft with no board that the manager edited: it cannot be saved. */
export function isWatchesInvalid(draft: ControlDraft, stored: ControlDraft): boolean {
  return (
    draft.watches.scope === "selected" &&
    draft.watches.workflowIds.length === 0 &&
    isWatchesDirty(draft, stored)
  );
}

/**
 * The PUT body: only the members that differ from stored; the policy names all
 * actions the server named and carries `eligibility` only when it was edited.
 */
export function buildPutRequest(draft: ControlDraft, stored: ControlDraft): PutSettingsRequest {
  const request: PutSettingsRequest = {};
  if (isPolicyDirty(draft, stored)) {
    request.policy = { actions: { ...draft.actions } };
    if (draft.eligibility && isEligibilityDirty(draft, stored)) {
      request.policy.eligibility = { ...draft.eligibility };
    }
  }
  if (isWatchesDirty(draft, stored)) {
    request.watches =
      draft.watches.scope === "all"
        ? { scope: "all" }
        : { scope: "selected", workflow_ids: [...draft.watches.workflowIds] };
  }
  if (draft.projects && isProjectsDirty(draft, stored)) {
    request.projects = projectsRequest(draft.projects);
  }
  return request;
}

/** Copies one drafted action into `actions`; an action the draft does not name stays as it is. */
function keepAction(actions: PolicyActions, from: ControlDraft, action: SettingsAction) {
  const value = from.actions[action];
  if (value !== undefined) actions[action] = value;
}

/**
 * Folds a re-read stored value into the draft without discarding an edit:
 * an action the manager changed keeps its draft value, every other action
 * follows the new stored value, and Watches is one unit.
 */
export function mergeStored(
  draft: ControlDraft,
  oldStored: ControlDraft,
  newStored: ControlDraft,
): ControlDraft {
  const actions = { ...newStored.actions };
  for (const action of SETTINGS_ACTIONS) {
    if (draft.actions[action] !== oldStored.actions[action]) keepAction(actions, draft, action);
  }
  const watches = sameWatches(draft.watches, oldStored.watches) ? newStored.watches : draft.watches;
  const projects = sameProjects(draft.projects, oldStored.projects)
    ? newStored.projects
    : draft.projects;
  const eligibility = sameEligibility(draft.eligibility, oldStored.eligibility)
    ? newStored.eligibility
    : draft.eligibility;
  return { actions, eligibility, watches, projects };
}

/** After a 200: members edited since the request was sent stay as drafted, the rest take the response. */
export function settleAfterSave(
  current: ControlDraft,
  sent: ControlDraft,
  response: ControlDraft,
): ControlDraft {
  const actions = { ...response.actions };
  for (const action of SETTINGS_ACTIONS) {
    if (current.actions[action] !== sent.actions[action]) keepAction(actions, current, action);
  }
  const watches = sameWatches(current.watches, sent.watches) ? response.watches : current.watches;
  const projects = sameProjects(current.projects, sent.projects)
    ? response.projects
    : current.projects;
  const eligibility = sameEligibility(current.eligibility, sent.eligibility)
    ? response.eligibility
    : current.eligibility;
  return { actions, eligibility, watches, projects };
}

export type SwitchOffResult =
  | { ok: true; watches: WatchesDraft; capped: boolean }
  | { ok: false; reason: "no-boards" };

/** Turning "watch every board" off starts from every listed board, at most 50, in workspace order. */
export function switchOffWatches(boardIds: readonly string[]): SwitchOffResult {
  if (boardIds.length === 0) return { ok: false, reason: "no-boards" };
  return {
    ok: true,
    watches: { scope: "selected", workflowIds: boardIds.slice(0, MAX_WATCHED_BOARDS) },
    capped: boardIds.length > MAX_WATCHED_BOARDS,
  };
}
