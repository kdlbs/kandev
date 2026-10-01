import { ApiError } from "@/lib/api/client";
import type {
  ControlAction,
  ControlSetting,
  SetupCoordinatorRequest,
} from "@/lib/api/domains/coordinator-api";
import {
  CONTROL_ACTIONS,
  MAX_WATCHED_BOARDS,
  MAX_WATCHED_PROJECTS,
  projectsRequest,
  type ProjectsDraft,
  type WatchesDraft,
} from "./control-draft";
import { EMPTY_GOAL_FORM, type GoalFormState } from "./goal-form";
import { COORDINATOR_NAME_MAX_LENGTH } from "./validate-form";

export const SETUP_STEPS = [
  "identity",
  "watches",
  "projects",
  "goal",
  "context",
  "may-do",
  "review",
] as const;
export type SetupStepId = (typeof SETUP_STEPS)[number];

/** The steps shown: Projects exists only while the phase 3.1 flag is effective. */
export function activeSetupSteps(projectsOffered: boolean): SetupStepId[] {
  return SETUP_STEPS.filter((step) => step !== "projects" || projectsOffered);
}

export const GOAL_NAME_MAX = 120;
export const GOAL_CRITERION_MAX = 200;
export const GOAL_CRITERIA_MAX = 10;
export const CONTEXT_MAX = 4000;
export const CONTEXT_REVIEW_CUT = 80;

export type SetupState = {
  name: string;
  agentProfileId: string;
  executorProfileId: string;
  watches: WatchesDraft;
  projects: ProjectsDraft;
  goal: GoalFormState;
  context: string;
  actions: Record<ControlAction, ControlSetting>;
};

export function initialSetupState(defaults: {
  agentProfileId: string;
  executorProfileId: string;
}): SetupState {
  const actions = {} as Record<ControlAction, ControlSetting>;
  for (const action of CONTROL_ACTIONS) {
    actions[action] =
      action === "start_agent" || action === "stop" ? "denied" : "requires_approval";
  }
  return {
    name: "",
    agentProfileId: defaults.agentProfileId,
    executorProfileId: defaults.executorProfileId,
    watches: { scope: "all", workflowIds: [] },
    projects: { scope: "all", entries: [], includeNoRepository: false },
    goal: EMPTY_GOAL_FORM,
    context: "",
    actions,
  };
}

function length(value: string): number {
  return Array.from(value).length;
}

/** A field path of the setup request; the key of one error slot. */
export type SetupFieldPath = string;

/** i18n key of a client-side message, or a server code resolved by the view. */
export type SetupErrors = Record<SetupFieldPath, string>;

const KEY = {
  name: "coordinator:setupErrorName",
  agent: "coordinator:setupErrorAgentProfile",
  executor: "coordinator:setupErrorExecutor",
  watches: "coordinator:watchesKeepOneBoard",
  watchesMax: "coordinator:watchesAtMost",
  projects: "coordinator:watchesKeepOneProject",
  projectsMax: "coordinator:watchesProjectsAtMost",
  goalName: "coordinator:setupErrorGoalName",
  goalDue: "coordinator:setupErrorGoalDue",
  criteriaMax: "coordinator:setupErrorCriteriaMax",
  criterion: "coordinator:setupErrorCriterion",
  context: "coordinator:setupErrorContext",
} as const;

export function identityErrors(state: SetupState): SetupErrors {
  const errors: SetupErrors = {};
  const name = state.name.trim();
  if (name === "" || length(name) > COORDINATOR_NAME_MAX_LENGTH) errors.name = KEY.name;
  if (state.agentProfileId === "") errors.agent_profile_id = KEY.agent;
  if (state.executorProfileId === "") errors.executor_profile_id = KEY.executor;
  return errors;
}

export function watchesErrors(state: SetupState): SetupErrors {
  const { scope, workflowIds } = state.watches;
  if (scope === "all") return {};
  if (workflowIds.length === 0) return { "watches.workflow_ids": KEY.watches };
  if (workflowIds.length > MAX_WATCHED_BOARDS) return { "watches.workflow_ids": KEY.watchesMax };
  return {};
}

export function projectsErrors(state: SetupState): SetupErrors {
  const { scope, entries, includeNoRepository } = state.projects;
  if (scope === "all") return {};
  if (entries.length === 0 && !includeNoRepository) return { "projects.entries": KEY.projects };
  if (entries.length > MAX_WATCHED_PROJECTS) return { "projects.entries": KEY.projectsMax };
  return {};
}

/** A goal with no name, no due date and no criteria row is the same as skipped. */
export function isGoalEmpty(goal: GoalFormState): boolean {
  return goal.name.trim() === "" && goal.dueOn === "" && goal.criteria.length === 0;
}

function isRealDate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return false;
  const [year, month, day] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
  );
}

export function goalErrors(goal: GoalFormState): SetupErrors {
  if (isGoalEmpty(goal)) return {};
  const errors: SetupErrors = {};
  const name = goal.name.trim();
  if (name === "" || length(name) > GOAL_NAME_MAX) errors["goal.name"] = KEY.goalName;
  if (goal.dueOn !== "" && !isRealDate(goal.dueOn)) errors["goal.due_on"] = KEY.goalDue;
  if (goal.criteria.length > GOAL_CRITERIA_MAX) errors["goal.criteria"] = KEY.criteriaMax;
  goal.criteria.forEach((criterion, index) => {
    const text = criterion.text.trim();
    if (text === "" || length(text) > GOAL_CRITERION_MAX) {
      errors[`goal.criteria[${index}].text`] = KEY.criterion;
    }
  });
  return errors;
}

export function contextErrors(context: string): SetupErrors {
  return length(context.trim()) > CONTEXT_MAX ? { context: KEY.context } : {};
}

export function stepErrors(step: SetupStepId, state: SetupState): SetupErrors {
  switch (step) {
    case "identity":
      return identityErrors(state);
    case "watches":
      return watchesErrors(state);
    case "projects":
      return projectsErrors(state);
    case "goal":
      return goalErrors(state.goal);
    case "context":
      return contextErrors(state.context);
    default:
      return {};
  }
}

export function isStepValid(step: SetupStepId, state: SetupState): boolean {
  return Object.keys(stepErrors(step, state)).length === 0;
}

export function isSetupValid(state: SetupState): boolean {
  return SETUP_STEPS.every((step) => isStepValid(step, state));
}

/** Skip clears the step's values. */
export function skipStep(step: SetupStepId, state: SetupState): SetupState {
  if (step === "projects") {
    return { ...state, projects: { scope: "all", entries: [], includeNoRepository: false } };
  }
  if (step === "goal") return { ...state, goal: EMPTY_GOAL_FORM };
  if (step === "context") return { ...state, context: "" };
  return state;
}

export function buildSetupRequest(state: SetupState): SetupCoordinatorRequest {
  const request: SetupCoordinatorRequest = {
    name: state.name.trim(),
    agent_profile_id: state.agentProfileId,
    executor_profile_id: state.executorProfileId,
    context: state.context.trim(),
    watches:
      state.watches.scope === "all"
        ? { scope: "all" }
        : { scope: "selected", workflow_ids: [...state.watches.workflowIds] },
    policy: { actions: { ...state.actions } },
  };
  if (state.projects.scope === "selected") request.projects = projectsRequest(state.projects);
  if (!isGoalEmpty(state.goal)) {
    request.goal = {
      name: state.goal.name.trim(),
      due_on: state.goal.dueOn === "" ? null : state.goal.dueOn,
      criteria: state.goal.criteria.map((c) => ({ text: c.text.trim() })),
    };
  }
  return request;
}

/** The text up to its first line break, trimmed and cut to 80 code points with "…" when cut. */
export function contextReviewValue(context: string): string | null {
  const trimmed = context.trim();
  if (trimmed === "") return null;
  const [first, ...rest] = trimmed.split(/\r\n|\r|\n/);
  const line = first.trim();
  const points = Array.from(line);
  if (points.length > CONTEXT_REVIEW_CUT) {
    return `${points.slice(0, CONTEXT_REVIEW_CUT).join("")}…`;
  }
  return rest.length > 0 ? `${line}…` : line;
}

export type SetupServerError = {
  step: SetupStepId | null;
  field: string | null;
  code: string | null;
};

const SERVER_STEPS: readonly string[] = [
  "identity",
  "watches",
  "projects",
  "goal",
  "context",
  "may-do",
];

/** A 400 carrying `step` is a setup validation failure; any other answer is not. */
export function setupServerError(error: unknown): SetupServerError | null {
  if (!(error instanceof ApiError) || error.status !== 400) return null;
  const body = error.body as { step?: unknown; field?: unknown; code?: unknown } | null;
  if (typeof body?.step !== "string" || !SERVER_STEPS.includes(body.step)) return null;
  return {
    step: body.step as SetupStepId,
    field: typeof body.field === "string" ? body.field : null,
    code: typeof body.code === "string" ? body.code : null,
  };
}

/** Whether the server answered with a refusal; a network failure or an unreadable 2xx body did not. */
export function gotRefusal(error: unknown): boolean {
  return error instanceof ApiError && error.status >= 400;
}

export const WATCHES_PATHS: readonly SetupFieldPath[] = ["watches.scope", "watches.workflow_ids"];

export const PROJECTS_PATHS: readonly SetupFieldPath[] = ["projects.scope", "projects.entries"];

export function actionPath(action: ControlAction): SetupFieldPath {
  return `policy.actions.${action}`;
}

const ACTION_PATH = /^policy\.actions\.(.+)$/;

/** The action a `policy.actions.<action>` path names, when it is one of the six. */
export function actionOfPath(path: string): ControlAction | null {
  const name = ACTION_PATH.exec(path)?.[1];
  return CONTROL_ACTIONS.find((a) => a === name) ?? null;
}
