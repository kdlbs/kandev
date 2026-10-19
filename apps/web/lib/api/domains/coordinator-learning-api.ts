import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import { coordinatorPath } from "@/lib/api/domains/coordinator-api";

// Mirrors internal/coordinator/learning.go and outcome_measures.go.
export type LearningHealthState = "off" | "waiting" | "running" | "fresh" | "stale" | "failed";

export type LearningHealth = {
  state: LearningHealthState;
  condition?: string;
  detail?: string;
};

export type LearningView = { shadow_dream: boolean; health: LearningHealth };

export type DreamSummary = {
  id: string;
  status: "running" | "ok" | "clean" | "partial" | "failed" | "skipped";
  reason?: string;
  window_start: string;
  window_end: string;
  turn_count: number;
  item_count: number;
  cost_subcents: number | null;
  started_at: string;
  finished_at: string | null;
};

export type DreamsPage = { dreams: DreamSummary[]; next_before?: string };

export type DreamRating = "useful" | "not_useful" | "harmful";

export type DreamItem = {
  id: string;
  position: number;
  kind: string;
  text: string;
  target_id?: string;
  cited_turn_ids: string[];
  gate: string;
  replay_id?: string;
  replay_verdict?: string;
  replay_reason?: string;
  rating?: DreamRating;
};

export type DreamDetail = DreamSummary & { considered: string[]; items: DreamItem[] };

export type OutcomeMeasure = {
  value: number | null;
  numerator: number;
  denominator: number;
  null_reason?: "no_data" | "cost_unknown" | "too_few";
  capped: boolean;
};

export type OutcomeMeasures = {
  days: number;
  approval_without_edit: OutcomeMeasure;
  override_recurrence: OutcomeMeasure;
  dollars_per_merged_task: OutcomeMeasure;
  median_wait_seconds: OutcomeMeasure;
  agreement: OutcomeMeasure;
};

const JSON_HEADERS = { "Content-Type": "application/json" };

export function getLearning(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<LearningView> {
  return fetchJson<LearningView>(coordinatorPath(workspaceId, coordinatorId, "/learning"), options);
}

export function putShadowDream(
  workspaceId: string,
  coordinatorId: string,
  shadowDream: boolean,
  options?: ApiRequestOptions,
): Promise<LearningView> {
  return fetchJson<LearningView>(coordinatorPath(workspaceId, coordinatorId, "/learning"), {
    ...options,
    init: {
      ...options?.init,
      method: "PUT",
      headers: { ...JSON_HEADERS, ...options?.init?.headers },
      body: JSON.stringify({ shadow_dream: shadowDream }),
    },
  });
}

export function getOutcomeMeasures(
  workspaceId: string,
  coordinatorId: string,
  days: number,
  options?: ApiRequestOptions,
): Promise<OutcomeMeasures> {
  return fetchJson<OutcomeMeasures>(
    coordinatorPath(workspaceId, coordinatorId, `/measures?days=${days}`),
    options,
  );
}

export function listDreams(
  workspaceId: string,
  coordinatorId: string,
  before?: string,
  options?: ApiRequestOptions,
): Promise<DreamsPage> {
  const query = before ? `?before=${encodeURIComponent(before)}` : "";
  return fetchJson<DreamsPage>(
    coordinatorPath(workspaceId, coordinatorId, `/dreams${query}`),
    options,
  );
}

export function getDream(
  workspaceId: string,
  coordinatorId: string,
  dreamId: string,
  options?: ApiRequestOptions,
): Promise<DreamDetail> {
  return fetchJson<DreamDetail>(
    coordinatorPath(workspaceId, coordinatorId, `/dreams/${encodeURIComponent(dreamId)}`),
    options,
  );
}

export type DreamRatingTarget = {
  workspaceId: string;
  coordinatorId: string;
  dreamId: string;
  itemId: string;
};

export function putDreamRating(
  { workspaceId, coordinatorId, dreamId, itemId }: DreamRatingTarget,
  rating: DreamRating,
  options?: ApiRequestOptions,
): Promise<unknown> {
  return fetchJson<unknown>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/dreams/${encodeURIComponent(dreamId)}/items/${encodeURIComponent(itemId)}/rating`,
    ),
    {
      ...options,
      init: {
        ...options?.init,
        method: "PUT",
        headers: { ...JSON_HEADERS, ...options?.init?.headers },
        body: JSON.stringify({ rating }),
      },
    },
  );
}
