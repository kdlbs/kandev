"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import { notifyWorkspaceAgentsChanged } from "@/hooks/domains/office/use-workspace-agents";
export function AgentDelegationContext({ agentId, initial }: { agentId: string; initial: string }) {
  const { t } = useTranslation();
  const [value, setValue] = useState(initial);
  const [saved, setSaved] = useState(initial);
  useSettingsSaveContributor({
    id: `delegation-${agentId}`,
    revision: value,
    isDirty: value !== saved,
    save: async () => {
      await updateAgentProfile(agentId, { delegationContext: value });
      setSaved(value);
      notifyWorkspaceAgentsChanged();
    },
    discard: () => setValue(saved),
  });
  return (
    <div className="space-y-2">
      <Label htmlFor={`delegation-${agentId}`}>{t("office:delegationContext")}</Label>
      <Textarea
        id={`delegation-${agentId}`}
        value={value}
        maxLength={2000}
        onChange={(e) => setValue(e.target.value)}
        placeholder={t("office:delegationContextExample")}
      />
      <p className="text-xs text-muted-foreground">{t("office:delegationContextHint")}</p>
    </div>
  );
}
