import { useCallback, useState } from "react";
import { listOrchestrators } from "@/lib/api/domains/orchestration-api";
import { selectAssistant, type AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useOrchestrationData } from "./use-orchestration-data";
export function useAssistantSetup(
  workspace: string,
  binding: AssistantBinding | null,
  onSelected: () => void,
) {
  const load = useCallback(
    () => (workspace ? listOrchestrators(workspace) : Promise.resolve({ orchestrators: [] })),
    [workspace],
  );
  const data = useOrchestrationData(load, binding?.owner_user_id);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const select = async (id: string) => {
    if (!id || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      await selectAssistant(id, binding?.version ?? 0, binding?.execution_mode ?? "inspect");
      onSelected();
    } catch (cause) {
      setError(cause);
    } finally {
      setBusy(false);
    }
  };
  return { ...data, busy, select, actionError: error };
}
