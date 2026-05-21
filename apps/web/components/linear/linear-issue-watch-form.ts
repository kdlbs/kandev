import type { LinearIssueWatch, LinearSearchFilter, LinearUser } from "@/lib/types/linear";
import { DEFAULT_LINEAR_ISSUE_WATCH_PROMPT } from "./linear-issue-watch-placeholders";

export const ASSIGNED_ANY = "__any__";
export const CREATOR_ANY = "__any__";

export type LinearPriority = 0 | 1 | 2 | 3 | 4;

// Linear priorities: 0=None, 1=Urgent, 2=High, 3=Medium, 4=Low. Rendered as
// toggle chips, mirroring the States and Labels multi-selects.
export const PRIORITY_OPTIONS: { value: LinearPriority; label: string }[] = [
  { value: 1, label: "Urgent" },
  { value: 2, label: "High" },
  { value: 3, label: "Medium" },
  { value: 4, label: "Low" },
  { value: 0, label: "No priority" },
];

export interface FormState {
  workspaceId: string;
  query: string;
  teamKey: string;
  stateIds: string[];
  assigned: string;
  priorities: LinearPriority[];
  labelIds: string[];
  creatorId: string;
  estimateMin: string;
  estimateMax: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollInterval: number;
}

export function makeEmptyForm(workspaceId: string): FormState {
  return {
    workspaceId,
    query: "",
    teamKey: "",
    stateIds: [],
    assigned: "",
    priorities: [],
    labelIds: [],
    creatorId: "",
    estimateMin: "",
    estimateMax: "",
    workflowId: "",
    workflowStepId: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: true,
    pollInterval: 300,
  };
}

function estimateString(v: number | null | undefined): string {
  return v === undefined || v === null ? "" : String(v);
}

export function formStateFromWatch(w: LinearIssueWatch): FormState {
  const f: LinearSearchFilter = w.filter ?? {};
  return {
    workspaceId: w.workspaceId,
    query: f.query ?? "",
    teamKey: f.teamKey ?? "",
    stateIds: f.stateIds ?? [],
    assigned: f.assigned ?? "",
    priorities: f.priorities ?? [],
    labelIds: f.labelIds ?? [],
    creatorId: f.creatorId ?? "",
    estimateMin: estimateString(f.estimateMin),
    estimateMax: estimateString(f.estimateMax),
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt || DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
  };
}

export function parseEstimate(raw: string): number | undefined {
  const t = raw.trim();
  if (t === "") return undefined;
  const n = Number(t);
  if (!Number.isFinite(n) || n < 0) return undefined;
  return n;
}

export function filterIsEmpty(form: FormState): boolean {
  return (
    form.query.trim() === "" &&
    form.teamKey.trim() === "" &&
    form.assigned.trim() === "" &&
    form.stateIds.length === 0 &&
    form.priorities.length === 0 &&
    form.labelIds.length === 0 &&
    form.creatorId.trim() === "" &&
    parseEstimate(form.estimateMin) === undefined &&
    parseEstimate(form.estimateMax) === undefined
  );
}

export function buildFilterPayload(form: FormState): LinearSearchFilter {
  return {
    query: form.query.trim() || undefined,
    teamKey: form.teamKey.trim() || undefined,
    stateIds: form.stateIds.length > 0 ? form.stateIds : undefined,
    assigned: form.assigned.trim() || undefined,
    priorities: form.priorities.length > 0 ? form.priorities : undefined,
    labelIds: form.labelIds.length > 0 ? form.labelIds : undefined,
    creatorId: form.creatorId.trim() || undefined,
    estimateMin: parseEstimate(form.estimateMin),
    estimateMax: parseEstimate(form.estimateMax),
  };
}

export function userOptionLabel(u: LinearUser): string {
  const name = u.displayName?.trim() || u.name?.trim() || u.email?.trim() || u.id;
  if (u.email && u.email !== name) return `${name} (${u.email})`;
  return name;
}

export function creatorPlaceholder(teamKey: string, loadingUsers: boolean): string {
  if (loadingUsers) return "Loading…";
  if (!teamKey) return "Pick a team first";
  return "(any)";
}
