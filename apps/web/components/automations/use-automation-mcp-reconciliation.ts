import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import { useAutomationMCPSelectionReload } from "@/hooks/domains/settings/use-automation-mcp-options";
import type { FormState } from "./automation-payload";

export function useAutomationMCPReconciliation(
  workspaceId: string,
  automationId: string | null,
  setForm: Dispatch<SetStateAction<FormState>>,
  setSavedForm: Dispatch<SetStateAction<FormState>>,
) {
  const { t } = useTranslation();
  const [saveFailed, setSaveFailed] = useState(false);
  const reloadSaved = useAutomationMCPSelectionReload(workspaceId, automationId);
  const reload = useCallback(async () => {
    try {
      const saved = await reloadSaved();
      if (!saved) return;
      setSavedForm((previous) => ({
        ...previous,
        mcpSelection: saved.mcp_selection,
        mcpSelectionRevision: saved.mcp_selection_revision ?? 0,
      }));
      setForm((previous) => ({
        ...previous,
        mcpSelectionRevision: saved.mcp_selection_revision ?? 0,
      }));
      setSaveFailed(false);
    } catch {
      toast.error(t("automations:mcpOptionsFailed"));
    }
  }, [reloadSaved, setForm, setSavedForm, t]);
  return {
    saveFailed,
    reload,
    failed: () => setSaveFailed(true),
    saved: () => setSaveFailed(false),
  };
}
