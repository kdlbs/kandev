import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { runMaintenance } from "@/lib/api/domains/assistant-maintenance-api";
import { generateUUID } from "@/lib/utils";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type {
  ImprovementDetail,
  MaintenanceAction,
  MaintenanceRequest,
} from "@/lib/api/domains/assistant-maintenance-types";
export function useMaintenanceMutations(
  binding: AssistantBinding,
  detail: ImprovementDetail,
  refresh: () => void,
) {
  const key = `${binding.owner_user_id}:${binding.id}:${binding.version}:${detail.candidate.id}`;
  const current = useRef(key);
  current.current = key;
  useEffect(
    () => () => {
      if (current.current === key) current.current = "";
    },
    [key],
  );
  const run = useMemo(
    () => ({ busy: false, attempt: undefined as MaintenanceRequest | undefined }),
    [key],
  );
  const [state, setState] = useState<{ key: string; busy: boolean; error?: unknown }>();
  const perform = async (fn: () => Promise<unknown>) => {
    if (run.busy) return false;
    run.busy = true;
    setState({ key, busy: true });
    try {
      await fn();
      if (current.current === key) {
        setState({ key, busy: false });
        refresh();
      }
      return true;
    } catch (error) {
      if (current.current === key) {
        setState({ key, busy: false, error });
        refresh();
      }
      return false;
    } finally {
      run.busy = false;
    }
  };
  const maintenance = (action: MaintenanceAction) =>
    perform(async () => {
      if (run.attempt && run.attempt.action !== action)
        throw new ApiError("operation_outcome_unknown", 409, {});
      run.attempt ??= {
        action,
        operation_id: generateUUID(),
        expected_binding_version: binding.version,
        expected_intent_revision: binding.intent_revision,
        candidate_revision: detail.candidate.revision,
        grant_revision: detail.grant?.revision ?? 0,
      };
      try {
        await runMaintenance(detail.candidate.id, run.attempt);
        run.attempt = undefined;
      } catch (error) {
        if (
          error instanceof ApiError &&
          error.status < 500 &&
          error.message !== "operation_outcome_unknown"
        )
          run.attempt = undefined;
        throw error;
      }
    });
  return {
    busy: state?.key === key && state.busy,
    error: state?.key === key ? state.error : undefined,
    maintenance,
    perform,
  };
}
