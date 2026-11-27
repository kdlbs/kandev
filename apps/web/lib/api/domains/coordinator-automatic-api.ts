import type { ApiRequestOptions } from "@/lib/api/client";
import {
  coordinatorPath,
  mutate,
  type ControlAction,
  type ControlSetting,
} from "@/lib/api/domains/coordinator-api";
import { fetchJson } from "@/lib/api/client";

export type EligibilityConditionName =
  | "history_30d"
  | "volume"
  | "unedited_rate"
  | "no_undo"
  | "reviewed_7d";

// Mirrors internal/coordinator/handlers_automatic.go's eligibilityResponse.
export type ClassEligibility = {
  eligible: boolean;
  conditions: {
    name: EligibilityConditionName | (string & {});
    met: boolean;
    value: number | string | null;
  }[];
  setting: ControlSetting;
  changed_by: string;
  changed_at: string | null;
};

export function getClassEligibility(
  workspaceId: string,
  coordinatorId: string,
  actionClass: ControlAction,
  options?: ApiRequestOptions,
): Promise<ClassEligibility> {
  return fetchJson<ClassEligibility>(
    coordinatorPath(workspaceId, coordinatorId, `/classes/${actionClass}/eligibility`),
    options,
  );
}

/** Records a class review; the server computes every stored field, so no body is sent. */
export function recordClassReview(
  workspaceId: string,
  coordinatorId: string,
  actionClass: ControlAction,
  options?: ApiRequestOptions,
): Promise<unknown> {
  return mutate<unknown>(
    coordinatorPath(workspaceId, coordinatorId, `/classes/${actionClass}/reviews`),
    "POST",
    undefined,
    options,
  );
}
