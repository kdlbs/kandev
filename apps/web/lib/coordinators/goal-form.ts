import type { Goal, GoalCriterion, PutGoalRequest } from "@/lib/api/domains/coordinator-api";

export type GoalFormCriterion = {
  key: string;
  id?: string;
  text: string;
  done: boolean;
};

export type GoalFormState = {
  name: string;
  dueOn: string;
  criteria: GoalFormCriterion[];
};

export const EMPTY_GOAL_FORM: GoalFormState = { name: "", dueOn: "", criteria: [] };

export function goalFormFromGoal(goal: Goal): GoalFormState {
  return {
    name: goal.name,
    dueOn: goal.due_on ?? "",
    criteria: goal.criteria.map((c) => ({ key: c.id, id: c.id, text: c.text, done: c.done })),
  };
}

function criteriaSignature(form: GoalFormState): string {
  return JSON.stringify(form.criteria.map((c) => [c.id ?? null, c.text]));
}

export function isGoalFormDirty(form: GoalFormState, saved: GoalFormState): boolean {
  return (
    form.name !== saved.name ||
    form.dueOn !== saved.dueOn ||
    criteriaSignature(form) !== criteriaSignature(saved)
  );
}

export function buildPutGoalRequest(
  form: GoalFormState,
  goalId: string | undefined,
): PutGoalRequest {
  const request: PutGoalRequest = {
    name: form.name.trim(),
    due_on: form.dueOn === "" ? null : form.dueOn,
    criteria: form.criteria.map((c) =>
      c.id ? { id: c.id, text: c.text.trim() } : { text: c.text.trim() },
    ),
  };
  return goalId ? { goal_id: goalId, ...request } : request;
}

// A refetch or toggle response updates only the done state of saved
// criteria; every unsaved edit in the form is kept.
export function withServerDoneStates(form: GoalFormState, goal: Goal): GoalFormState {
  const doneById = new Map(goal.criteria.map((c) => [c.id, c.done]));
  return {
    ...form,
    criteria: form.criteria.map((c) =>
      c.id && doneById.has(c.id) ? { ...c, done: doneById.get(c.id) as boolean } : c,
    ),
  };
}

export function metCriteriaCount(criteria: GoalCriterion[]): number {
  return criteria.filter((c) => c.done).length;
}

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

function localDateString(now: Date): string {
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

// due_on is a calendar date; ISO strings compare correctly as text. Today is
// the viewer's local calendar day.
export function isOverdue(dueOn: string | null, now: Date): boolean {
  return dueOn !== null && dueOn < localDateString(now);
}

const DATE_OPTIONS: Intl.DateTimeFormatOptions = {
  day: "numeric",
  month: "short",
  year: "numeric",
};

export function formatCalendarDate(dueOn: string, locale: string): string {
  const [year, month, day] = dueOn.split("-").map(Number);
  return new Intl.DateTimeFormat(locale, DATE_OPTIONS).format(new Date(year, month - 1, day));
}

export function formatTimestampDate(timestamp: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, DATE_OPTIONS).format(new Date(timestamp));
}
