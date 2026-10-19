"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { SettingsGroup } from "@/components/settings/settings-group";
import { useFeature } from "@/hooks/domains/features/use-feature";

type Props = {
  canManage: boolean;
  enabled: boolean;
  isDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
};

/**
 * Workspace opt-in for an automatic Try next after a dynamic route's provider
 * hits a capacity limit mid-turn. Off keeps the turn waiting for the user.
 */
export function WorkspaceDynamicRoutingCard({
  canManage,
  enabled,
  isDirty,
  onEnabledChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <SettingsGroup
      title={t("workspaces:dynamicRoutingPolicyTitle")}
      description={t("workspaces:dynamicRoutingPolicyDescription")}
      isDirty={isDirty}
      contentClassName="space-y-4 divide-y-0"
      data-testid="workspace-dynamic-routing-card"
    >
      <div className="flex min-h-11 items-center justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-midturn-failover" className="cursor-pointer">
            {t("workspaces:midTurnFailoverEnabled")}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:midTurnFailoverDescription")}
          </p>
        </div>
        <Switch
          id="workspace-midturn-failover"
          checked={enabled}
          onCheckedChange={onEnabledChange}
          disabled={!canManage}
          data-settings-dirty={isDirty}
          data-testid="workspace-midturn-failover-switch"
          className="[@media(pointer:coarse)]:after:-inset-y-3"
        />
      </div>
    </SettingsGroup>
  );
}

type SectionProps = {
  canManage: boolean;
  enabled: boolean;
  savedEnabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
};

/** Shows the card only while dynamic agent routing is available. */
export function WorkspaceDynamicRoutingSection({
  canManage,
  enabled,
  savedEnabled,
  onEnabledChange,
}: SectionProps) {
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  if (!dynamicRoutingEnabled) return null;
  return (
    <>
      <WorkspaceDynamicRoutingCard
        canManage={canManage}
        enabled={enabled}
        isDirty={enabled !== savedEnabled}
        onEnabledChange={onEnabledChange}
      />
      <Separator />
    </>
  );
}
