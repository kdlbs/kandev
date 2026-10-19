"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { SettingsGroup } from "@/components/settings/settings-group";
import { useFeature } from "@/hooks/domains/features/use-feature";

/** Draft of the automatic failed-route retry controls. */
export type FailedRouteAutoRetryControls = {
  enabled: boolean;
  max: string;
  maxValid: boolean;
  enabledIsDirty: boolean;
  maxIsDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onMaxChange: (value: string) => void;
};

type Props = {
  canManage: boolean;
  enabled: boolean;
  isDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  autoRetry?: FailedRouteAutoRetryControls;
};

/**
 * Workspace opt-ins for dynamic route recovery. The mid-turn failover toggle
 * tries the next candidate after a provider limit stops a turn; the
 * failed-route retry toggle retries a route left waiting for the user once
 * its providers recover. Both are off by default and independent.
 */
export function WorkspaceDynamicRoutingCard({
  canManage,
  enabled,
  isDirty,
  onEnabledChange,
  autoRetry,
}: Props) {
  const { t } = useTranslation();
  const autoRetryDirty = Boolean(autoRetry && (autoRetry.enabledIsDirty || autoRetry.maxIsDirty));

  return (
    <SettingsGroup
      title={t("workspaces:dynamicRoutingPolicyTitle")}
      description={t("workspaces:dynamicRoutingPolicyDescription")}
      isDirty={isDirty || autoRetryDirty}
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
      {autoRetry && <FailedRouteAutoRetryFields canManage={canManage} controls={autoRetry} />}
    </SettingsGroup>
  );
}

function FailedRouteAutoRetryFields({
  canManage,
  controls,
}: {
  canManage: boolean;
  controls: FailedRouteAutoRetryControls;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex min-h-11 items-center justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-failed-route-auto-retry" className="cursor-pointer">
            {t("workspaces:failedRouteAutoRetryEnabled")}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:failedRouteAutoRetryDescription")}
          </p>
        </div>
        <Switch
          id="workspace-failed-route-auto-retry"
          checked={controls.enabled}
          onCheckedChange={controls.onEnabledChange}
          disabled={!canManage}
          data-settings-dirty={controls.enabledIsDirty}
          data-testid="workspace-failed-route-auto-retry-switch"
          className="[@media(pointer:coarse)]:after:-inset-y-3"
        />
      </div>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center sm:gap-4">
        <Label htmlFor="workspace-failed-route-auto-retry-max">
          {t("workspaces:failedRouteAutoRetryMax")}
        </Label>
        <Input
          id="workspace-failed-route-auto-retry-max"
          type="number"
          min={1}
          max={3}
          step={1}
          value={controls.max}
          disabled={!canManage || !controls.enabled}
          aria-invalid={!controls.maxValid}
          aria-describedby={
            !controls.maxValid ? "workspace-failed-route-auto-retry-max-error" : undefined
          }
          data-settings-dirty={controls.maxIsDirty}
          data-testid="workspace-failed-route-auto-retry-max"
          controlSize="compact"
          className="text-base sm:text-sm md:text-xs/relaxed [@media(pointer:coarse)]:min-h-11"
          onChange={(event) => controls.onMaxChange(event.target.value)}
        />
        {!controls.maxValid && (
          <p
            id="workspace-failed-route-auto-retry-max-error"
            role="alert"
            className="text-sm text-destructive sm:col-start-2"
            data-testid="workspace-failed-route-auto-retry-max-error"
          >
            {t("workspaces:failedRouteAutoRetryMaxRange")}
          </p>
        )}
      </div>
    </>
  );
}

type SectionProps = {
  canManage: boolean;
  enabled: boolean;
  savedEnabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  autoRetry: FailedRouteAutoRetryControls;
};

/** Shows the card only while dynamic agent routing is available. */
export function WorkspaceDynamicRoutingSection({
  canManage,
  enabled,
  savedEnabled,
  onEnabledChange,
  autoRetry,
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
        autoRetry={autoRetry}
      />
      <Separator />
    </>
  );
}
