import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { listOrchestrators, type Orchestrator } from "@/lib/api/domains/orchestration-api";

export function OrchestratorTargetField({
  workspaceId,
  value,
  onChange,
}: {
  workspaceId: string;
  value: string;
  onChange: (id: string) => void;
}) {
  const enabled = useFeature("orchestration");
  const { t } = useTranslation();
  const [items, setItems] = useState<Orchestrator[]>([]);
  const [error, setError] = useState<string>();
  useEffect(() => {
    let active = true;
    setItems([]);
    setError(undefined);
    if (enabled)
      void listOrchestrators(workspaceId)
        .then((data) => {
          if (active) setItems(data.orchestrators);
        })
        .catch((e) => {
          if (active) setError(String(e));
        });
    return () => {
      active = false;
    };
  }, [workspaceId, enabled]);
  if (!enabled && !value) return null;
  return (
    <div className="space-y-2">
      <Label htmlFor="automation-target">{t("automations:runTarget")}</Label>
      <Select value={value || "agent"} onValueChange={(id) => onChange(id === "agent" ? "" : id)}>
        <SelectTrigger id="automation-target" data-testid="automation-target">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="agent">{t("automations:agentRunTarget")}</SelectItem>
          {value && !items.some((item) => item.id === value) && (
            <SelectItem value={value} disabled>
              {t("automations:orchestratorUnavailable")}
            </SelectItem>
          )}
          {items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error && <p role="alert">{error}</p>}
      {value && (
        <p className="text-xs text-muted-foreground">{t("automations:orchestratorTargetHint")}</p>
      )}
      {enabled && (
        <Link
          className="text-sm underline"
          href={`/settings/workspaces/${workspaceId}/orchestration`}
        >
          {t("automations:manageOrchestrators")}
        </Link>
      )}
    </div>
  );
}
