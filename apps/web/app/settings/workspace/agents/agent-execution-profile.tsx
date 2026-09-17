"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import type {
  AgentRoutingOverrides,
  ExecutionProfileSummary,
} from "@/lib/state/slices/office/types";

export function AgentExecutionProfile({
  agentId,
  workspaceId,
  initialProfileId,
}: {
  agentId: string;
  workspaceId: string;
  initialProfileId?: string;
}) {
  const { t } = useTranslation();
  const route = useAgentRoute(agentId);
  const routing = useWorkspaceRouting(workspaceId);
  if (!route.data && !route.error)
    return <p role={route.error ? "alert" : undefined}>{route.error ?? t("common:loading")}</p>;
  return (
    <ProfileSelection
      agentId={agentId}
      initial={route.data?.overrides?.execution_profile_id ?? initialProfileId ?? ""}
      profiles={routing.executionProfiles}
      save={(id) =>
        route.updateOverrides({
          ...route.data?.overrides,
          execution_profile_id: id,
        } as AgentRoutingOverrides)
      }
    />
  );
}
function ProfileSelection({
  agentId,
  initial,
  profiles,
  save,
}: {
  agentId: string;
  initial: string;
  profiles: ExecutionProfileSummary[];
  save: (id: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(initial);
  const [saved, setSaved] = useState(initial);
  useSettingsSaveContributor({
    id: `profile-${agentId}`,
    revision: value,
    isDirty: value !== saved,
    canSave: profiles.some((p) => p.id === value),
    save: async () => {
      await save(value);
      setSaved(value);
    },
    discard: () => setValue(saved),
  });
  return (
    <div className="space-y-2">
      <Label>{t("office:executionProfileChoice")}</Label>
      <Select value={value || "__inherit__"} onValueChange={setValue}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {!value && (
            <SelectItem value="__inherit__" disabled>
              {t("office:inherit")}
            </SelectItem>
          )}
          {value && !profiles.some((p) => p.id === value) && (
            <SelectItem value={value} disabled>
              {value}
            </SelectItem>
          )}
          {profiles.map((p) => (
            <SelectItem key={p.id} value={p.id}>
              {p.name} ({p.provider_id}, {p.model})
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("office:executionProfileChoiceHint")}</p>
      <Link className="text-xs underline cursor-pointer" href="/settings/agents">
        {t("office:manageExecutionProfiles")}
      </Link>
    </div>
  );
}
