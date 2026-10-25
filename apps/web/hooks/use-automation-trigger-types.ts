import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listTriggerTypes } from "@/lib/api/domains/automation-api";
import type { TriggerTypeInfo } from "@/lib/types/automation";

const EMPTY_TYPES: TriggerTypeInfo[] = [];

export function useTriggerTypeMetadata(workspaceId: string) {
  const types = useAppStore(
    (state) => state.automations.triggerTypes[workspaceId]?.items ?? EMPTY_TYPES,
  );
  const beginTriggerTypes = useAppStore((state) => state.beginTriggerTypes);
  const finishTriggerTypes = useAppStore((state) => state.finishTriggerTypes);
  useEffect(() => {
    const generation = beginTriggerTypes(workspaceId);
    if (generation === null) return;
    listTriggerTypes(workspaceId)
      .then((items) => finishTriggerTypes(workspaceId, generation, items))
      .catch(() => finishTriggerTypes(workspaceId, generation, []));
  }, [workspaceId, beginTriggerTypes, finishTriggerTypes]);
  return types;
}
