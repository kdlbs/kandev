import { useEffect, useRef, useState } from "react";
import { setAgentAutomaticUpdates, type AgentUpdateStatus } from "@/lib/api";
import { useAppStoreApi } from "@/components/state-provider";
import { refreshRuntimeUpdateStatuses } from "@/lib/agents/runtime-update-statuses";
import { t } from "@/lib/i18n";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function useRuntimeAutoUpdatePolicy(status: AgentUpdateStatus) {
  const store = useAppStoreApi();
  const [saved, setSaved] = useState(status.auto_update);
  const [draft, setDraft] = useState(status.auto_update);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const saving = useRef(false);
  const identity = useRef(status.runtime_id);
  useEffect(() => {
    if (identity.current !== status.runtime_id) {
      identity.current = status.runtime_id;
      setSaved(status.auto_update);
      setDraft(status.auto_update);
      return;
    }
    if (saving.current) return;
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(status.auto_update);
      return status.auto_update;
    });
  }, [status.auto_update, status.runtime_id]);
  const isDirty = draft !== saved;
  useSettingsSaveContributor({
    id: `runtime-automatic-${status.agent_name}`,
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(Number(revision));
      const runtimeID = status.runtime_id;
      saving.current = true;
      try {
        await setAgentAutomaticUpdates(status.agent_name, submitted);
        const refreshed = await refreshRuntimeUpdateStatuses(store, true);
        if (identity.current !== runtimeID) return;
        const accepted = refreshed
          ? (store.getState().agentRuntimeUpdates.byAgent[status.agent_name]?.auto_update ??
            submitted)
          : submitted;
        setSaved(accepted);
        if (draftRef.current === submitted) setDraft(accepted);
      } catch {
        throw new Error(t("agents:runtimePolicySaveFailed"));
      } finally {
        saving.current = false;
      }
    },
    discard: () => setDraft(saved),
  });
  return { draft, setDraft, isDirty };
}
