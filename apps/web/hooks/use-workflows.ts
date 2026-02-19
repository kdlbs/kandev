import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkflows } from "@/lib/api";

export function useWorkflows(workspaceId: string | null, enabled = true) {
  const workflows = useAppStore((state) => state.workflows.items);
  const setWorkflows = useAppStore((state) => state.setWorkflows);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    listWorkflows(workspaceId, { cache: "no-store" })
      .then((response) => {
        const mapped = response.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description,
        }));
        setWorkflows(mapped);
      })
      .catch(() => setWorkflows([]));
  }, [enabled, setWorkflows, workspaceId]);

  return { workflows };
}
