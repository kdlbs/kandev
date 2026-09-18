import { useRef, useState } from "react";
import { generateUUID } from "@/lib/utils";
import {
  controlAssistant,
  selectAssistant,
  type AssistantBinding,
  type AssistantControl,
  type AssistantControlReceipt,
  type AssistantMode,
} from "@/lib/api/domains/assistant-api";
import { ApiError } from "@/lib/api/client";
export function assistantFailureKey(error: unknown) {
  if (
    error instanceof ApiError &&
    error.status === 409 &&
    error.message !== "operation_outcome_unknown"
  )
    return "orchestration:assistantConflict";
  if (error instanceof ApiError && [401, 403, 404, 422].includes(error.status))
    return "orchestration:assistantActionUnavailable";
  return "orchestration:assistantOutcomeUnknown";
}
export function useAssistantActions(binding: AssistantBinding, refresh: () => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [receipt, setReceipt] = useState<AssistantControlReceipt>();
  const attempt = useRef<AssistantControl | undefined>(undefined);
  const inflight = useRef(false);
  const control = async (action: AssistantControl["action"], after = "") => {
    if (inflight.current) return;
    inflight.current = true;
    setBusy(true);
    setError(undefined);
    if (!attempt.current || attempt.current.action !== action || attempt.current.after !== after) {
      attempt.current = {
        action,
        after,
        operation_id: generateUUID(),
        expected_binding_version: binding.version,
        expected_intent_revision: after
          ? (receipt?.intent_revision ?? binding.intent_revision)
          : binding.intent_revision,
      };
    }
    try {
      setReceipt(await controlAssistant(attempt.current));
      attempt.current = undefined;
      refresh();
    } catch (cause) {
      if (
        cause instanceof ApiError &&
        cause.status < 500 &&
        cause.message !== "operation_outcome_unknown"
      )
        attempt.current = undefined;
      setError(cause);
      refresh();
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  };
  const mode = async (value: AssistantMode) => {
    if (inflight.current || value === binding.execution_mode) return;
    inflight.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await selectAssistant(binding.orchestrator_id, binding.version, value);
      refresh();
    } catch (cause) {
      setError(cause);
      refresh();
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  };
  return { busy, error, receipt, control, mode };
}
