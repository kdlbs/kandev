"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Switch } from "@kandev/ui/switch";
import {
  PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE,
  PERMISSION_KEYS,
  readPermissionValue,
} from "@/lib/agent-permissions";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import type { PermissionSetting, PassthroughConfig } from "@/lib/types/http";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

export type PermissionToggleProps = {
  agentName?: string;
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  variant: "default" | "compact";
  lockPassthrough?: boolean;
};

function permissionToggleWrapperClass(isDanger: boolean, compact: boolean): string {
  if (isDanger) {
    return "flex items-center justify-between gap-3 rounded-md border border-destructive/40 bg-destructive/5 p-3";
  }
  if (compact) {
    return "flex items-center justify-between gap-2";
  }
  return "flex items-center justify-between rounded-md border p-3";
}

function PermissionToggleRow({
  settingKey,
  setting,
  checked,
  onCheckedChange,
  compact,
  isDirty,
}: {
  settingKey: string;
  setting: PermissionSetting;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  compact: boolean;
  isDirty: boolean;
}) {
  const isDanger = setting.apply_method === PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE;
  const switchSize = compact ? ("sm" as const) : ("default" as const);
  const labelCls = compact ? "text-xs" : undefined;
  const wrapperCls = permissionToggleWrapperClass(isDanger, compact);
  const instanceId = useId();
  const switchId = `${instanceId}-permission-toggle-${settingKey}`;

  return (
    <div
      key={settingKey}
      className={wrapperCls}
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
      data-testid={isDanger ? "permission-auto-approve-danger" : `permission-toggle-${settingKey}`}
    >
      <div className={`flex-1 min-w-0 ${compact && !isDanger ? "space-y-0.5" : "space-y-1"}`}>
        <SettingsFieldLabel
          htmlFor={switchId}
          className={`flex items-center gap-1.5 ${labelCls ?? ""}`}
        >
          {isDanger && <IconAlertTriangle className="size-4 shrink-0 text-destructive" />}
          {setting.label}
        </SettingsFieldLabel>
        <SettingsFieldDescription>{setting.description}</SettingsFieldDescription>
      </div>
      <Switch id={switchId} size={switchSize} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}

export function PermissionToggles({
  agentName,
  profile,
  onChange,
  permissionSettings,
  passthroughConfig,
  variant,
  lockPassthrough,
  baselineProfile,
}: PermissionToggleProps) {
  const { t } = useTranslation();
  const passthroughLabel =
    agentName === "minimax-acp" ? t("agents:minimaxPassthroughLabel") : passthroughConfig?.label;
  const passthroughDescription =
    agentName === "minimax-acp"
      ? t("agents:minimaxPassthroughDescription")
      : passthroughConfig?.description;
  const isCompact = variant === "compact";
  if (isCompact) {
    return (
      <>
        {PERMISSION_KEYS.map((key) => {
          const setting = permissionSettings[key];
          if (!setting?.supported) return null;
          if (setting.apply_method === "cli_flag") return null;
          const checked = readPermissionValue(profile, key, permissionSettings);
          return (
            <PermissionToggleRow
              key={key}
              settingKey={key}
              setting={setting}
              checked={checked}
              onCheckedChange={(checked) => onChange({ [key]: checked })}
              compact
              isDirty={
                Boolean(baselineProfile) &&
                checked !== readPermissionValue(baselineProfile!, key, permissionSettings)
              }
            />
          );
        })}
        {passthroughConfig?.supported && (
          <div className="flex items-center justify-between gap-2">
            <div className="space-y-0.5">
              <SettingsFieldLabel className="text-xs">{passthroughLabel}</SettingsFieldLabel>
              <SettingsFieldDescription>{passthroughDescription}</SettingsFieldDescription>
            </div>
            <Switch
              size="sm"
              checked={profile.cli_passthrough}
              disabled={lockPassthrough}
              onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
            />
          </div>
        )}
      </>
    );
  }
  return (
    <>
      {PERMISSION_KEYS.map((key) => {
        const setting = permissionSettings[key];
        if (!setting?.supported) return null;
        if (setting.apply_method === "cli_flag") return null;
        const checked = readPermissionValue(profile, key, permissionSettings);
        return (
          <PermissionToggleRow
            key={key}
            settingKey={key}
            setting={setting}
            checked={checked}
            onCheckedChange={(checked) => onChange({ [key]: checked })}
            compact={false}
            isDirty={
              Boolean(baselineProfile) &&
              checked !== readPermissionValue(baselineProfile!, key, permissionSettings)
            }
          />
        );
      })}

      {passthroughConfig?.supported && (
        <div
          className="flex items-center justify-between rounded-md border p-3"
          data-testid="cli-passthrough-toggle"
        >
          <div className="space-y-1">
            <SettingsFieldLabel>{passthroughLabel}</SettingsFieldLabel>
            <SettingsFieldDescription>{passthroughDescription}</SettingsFieldDescription>
          </div>
          <Switch
            size="default"
            checked={profile.cli_passthrough}
            disabled={lockPassthrough}
            onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
          />
        </div>
      )}
    </>
  );
}
