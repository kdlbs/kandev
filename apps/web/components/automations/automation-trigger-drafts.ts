import { useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import {
  addTrigger,
  deleteTrigger,
  getAutomation,
  updateTrigger,
} from "@/lib/api/domains/automation-api";
import type { AutomationTrigger, TriggerType } from "@/lib/types/automation";
import { generateUUID } from "@/lib/utils";
import type { PendingTrigger } from "./automation-payload";

function pendingToTrigger(pending: PendingTrigger): AutomationTrigger {
  return {
    id: pending.tempId,
    automation_id: "",
    type: pending.type,
    config: pending.config,
    enabled: pending.enabled,
    last_evaluated_at: null,
    created_at: "",
    updated_at: "",
  };
}

export function useAutomationTriggerDrafts(currentId: string | null) {
  const [triggers, setTriggers] = useState<AutomationTrigger[]>([]);
  const [baselineTriggers, setBaselineTriggers] = useState<AutomationTrigger[]>([]);
  const [pending, setPending] = useState<PendingTrigger[]>([]);
  const latestPendingRef = useRef(pending);
  latestPendingRef.current = pending;

  const loadTriggers = (next: AutomationTrigger[]) => {
    setTriggers(next);
    setBaselineTriggers(next);
    setPending([]);
  };

  const handleAdd = async (type: TriggerType, config: Record<string, unknown>) => {
    setPending((items) => [...items, { tempId: generateUUID(), type, config, enabled: true }]);
  };

  const updateDraft = (triggerId: string, update: Partial<AutomationTrigger>) => {
    if (pending.some((trigger) => trigger.tempId === triggerId)) {
      setPending((items) =>
        items.map((trigger) =>
          trigger.tempId === triggerId ? { ...trigger, ...update } : trigger,
        ),
      );
      return;
    }
    setTriggers((items) =>
      items.map((trigger) => (trigger.id === triggerId ? { ...trigger, ...update } : trigger)),
    );
  };

  const handleUpdate = async (triggerId: string, config: Record<string, unknown>) => {
    updateDraft(triggerId, { config });
  };

  const handleToggle = async (triggerId: string, enabled: boolean) => {
    updateDraft(triggerId, { enabled });
  };

  const handleDelete = async (triggerId: string) => {
    setPending((items) => items.filter((trigger) => trigger.tempId !== triggerId));
    setTriggers((items) => items.filter((trigger) => trigger.id !== triggerId));
  };

  const allTriggers = [...triggers, ...pending.map(pendingToTrigger)];
  const persistDrafts = async (): Promise<AutomationTrigger[]> => {
    if (!currentId) return allTriggers;
    await deleteRemovedTriggers(baselineTriggers, triggers, setBaselineTriggers);
    await updateChangedTriggers(baselineTriggers, triggers, setBaselineTriggers);
    await createPendingTriggers({
      automationId: currentId,
      pending,
      latestPendingRef,
      setPending,
      setTriggers,
      setBaseline: setBaselineTriggers,
    });
    const refreshed = await getAutomation(currentId);
    setBaselineTriggers(refreshed.triggers ?? []);
    return refreshed.triggers ?? [];
  };

  return {
    loadTriggers,
    baselineTriggers,
    pending,
    clearPending: () => setPending([]),
    discardDrafts: () => {
      setTriggers(baselineTriggers);
      setPending([]);
    },
    persistDrafts,
    allTriggers,
    handleAdd,
    handleUpdate,
    handleToggle,
    handleDelete,
  };
}

async function deleteRemovedTriggers(
  baseline: AutomationTrigger[],
  current: AutomationTrigger[],
  setBaseline: Dispatch<SetStateAction<AutomationTrigger[]>>,
) {
  const currentIds = new Set(current.map((trigger) => trigger.id));
  for (const trigger of baseline) {
    if (currentIds.has(trigger.id)) continue;
    await deleteTrigger(trigger.id);
    setBaseline((items) => items.filter((item) => item.id !== trigger.id));
  }
}

async function updateChangedTriggers(
  baseline: AutomationTrigger[],
  current: AutomationTrigger[],
  setBaseline: Dispatch<SetStateAction<AutomationTrigger[]>>,
) {
  for (const trigger of current) {
    const saved = baseline.find((item) => item.id === trigger.id);
    if (!saved || triggerDraftEquals(trigger, saved)) continue;
    await updateTrigger(trigger.id, { config: trigger.config, enabled: trigger.enabled });
    setBaseline((items) => items.map((item) => (item.id === trigger.id ? trigger : item)));
  }
}

interface CreatePendingTriggersOptions {
  automationId: string;
  pending: PendingTrigger[];
  latestPendingRef: MutableRefObject<PendingTrigger[]>;
  setPending: Dispatch<SetStateAction<PendingTrigger[]>>;
  setTriggers: Dispatch<SetStateAction<AutomationTrigger[]>>;
  setBaseline: Dispatch<SetStateAction<AutomationTrigger[]>>;
}

async function createPendingTriggers({
  automationId,
  pending,
  latestPendingRef,
  setPending,
  setTriggers,
  setBaseline,
}: CreatePendingTriggersOptions) {
  for (const draft of pending) {
    const created = await addTrigger({
      automation_id: automationId,
      type: draft.type,
      config: draft.config,
      enabled: draft.enabled,
    });
    const latestDraft = latestPendingRef.current.find((item) => item.tempId === draft.tempId);
    setPending((items) => items.filter((item) => item.tempId !== draft.tempId));
    if (latestDraft) {
      setTriggers((items) => [
        ...items,
        { ...created, config: latestDraft.config, enabled: latestDraft.enabled },
      ]);
    }
    setBaseline((items) => [...items, created]);
  }
}

function triggerDraftEquals(left: AutomationTrigger, right: AutomationTrigger): boolean {
  return (
    left.enabled === right.enabled && JSON.stringify(left.config) === JSON.stringify(right.config)
  );
}
