import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkflows } from "@/lib/api";

export function useWorkflows(workspaceId: string | null, enabled = true) {
  const workflows = useAppStore((state) => state.workflows.items);
  const setWorkflows = useAppStore((state) => state.setWorkflows);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    let cancelled = false;
    listWorkflows(workspaceId, { cache: "no-store", includeHidden: true })
      .then((response) => {
        if (cancelled) return;
        const mapped = response.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description,
          sortOrder: workflow.sort_order ?? 0,
          agent_profile_id: workflow.agent_profile_id,
          hidden: workflow.hidden,
          style: workflow.style,
        }));
        setWorkflows(mapped);
      })
      .catch(() => {
        if (cancelled) return;
        setWorkflows([]);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, setWorkflows, workspaceId]);

  return { workflows };
}
