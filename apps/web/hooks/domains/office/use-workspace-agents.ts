import { useCallback, useEffect, useRef, useState } from "react";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { loadWorkspaceAgents, setWorkspaceChief } from "@/lib/api/domains/workspace-agents-api";

const CHANGE_EVENT = "kandev:workspace-agents-changed";
export function notifyWorkspaceAgentsChanged() {
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

export function useWorkspaceAgents(workspaceId: string | null) {
  const [result, setResult] = useState<
    Awaited<ReturnType<typeof loadWorkspaceAgents>> & { workspaceId: string }
  >();
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const refresh = useCallback(async () => {
    const version = ++generation.current;
    if (!workspaceId) return;
    setError(null);
    try {
      const data = await loadWorkspaceAgents(workspaceId);
      if (version === generation.current) setResult({ ...data, workspaceId });
    } catch (e) {
      if (version === generation.current) setError(e instanceof Error ? e.message : String(e));
    }
  }, [workspaceId]);
  useEffect(() => {
    void refresh();
    window.addEventListener(CHANGE_EVENT, refresh);
    return () => {
      generation.current++;
      window.removeEventListener(CHANGE_EVENT, refresh);
    };
  }, [refresh]);
  useOfficeRefetch("agents", refresh);
  const selectChief = async (agentId: string) => {
    if (!workspaceId) return;
    await setWorkspaceChief(workspaceId, agentId);
    notifyWorkspaceAgentsChanged();
  };
  const data = result?.workspaceId === workspaceId ? result : undefined;
  return {
    agents: data?.agents ?? [],
    chiefId: data?.chiefId ?? "",
    loading: !!workspaceId && !data && !error,
    error,
    refresh,
    selectChief,
  };
}
