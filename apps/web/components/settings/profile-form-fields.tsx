"use client";

import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { ModelCombobox } from "@/components/settings/model-combobox";
import { PERMISSION_KEYS, type PermissionKey } from "@/lib/agent-permissions";
import type { ModelConfig, PermissionSetting, PassthroughConfig } from "@/lib/types/http";

export type ProfileFormData = {
  name: string;
  model: string;
  mode: string;
  cli_passthrough: boolean;
} & Record<PermissionKey, boolean>;

export type ProfileFormFieldsProps = {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  agentName: string;
  onRemove?: () => void;
  canRemove?: boolean;
  variant?: "default" | "compact";
  hideNameField?: boolean;
  lockPassthrough?: boolean;
};

type PermissionToggleProps = {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  variant: "default" | "compact";
  lockPassthrough?: boolean;
};

function PermissionToggles({
  profile,
  onChange,
  permissionSettings,
  passthroughConfig,
  variant,
  lockPassthrough,
}: PermissionToggleProps) {
  const isCompact = variant === "compact";
  const switchSize = isCompact ? ("sm" as const) : ("default" as const);

  if (isCompact) {
    return (
      <>
        {PERMISSION_KEYS.map((key) => {
          const setting = permissionSettings[key];
          if (!setting?.supported) return null;
          return (
            <div key={key} className="flex items-center justify-between gap-2">
              <div className="space-y-0.5">
                <Label className="text-xs">{setting.label}</Label>
                <p className="text-[10px] text-muted-foreground leading-tight">
                  {setting.description}
                </p>
              </div>
              <Switch
                size={switchSize}
                checked={profile[key]}
                onCheckedChange={(checked) => onChange({ [key]: checked })}
              />
            </div>
          );
        })}
        {passthroughConfig?.supported && (
          <div className="flex items-center justify-between gap-2">
            <div className="space-y-0.5">
              <Label className="text-xs">{passthroughConfig.label}</Label>
              <p className="text-[10px] text-muted-foreground leading-tight">
                {passthroughConfig.description}
              </p>
            </div>
            <Switch
              size={switchSize}
              checked={profile.cli_passthrough}
              onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
              disabled={lockPassthrough}
            />
          </div>
        )}
      </>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {PERMISSION_KEYS.map((key) => {
        const setting = permissionSettings[key];
        if (!setting?.supported) return null;
        return (
          <div key={key} className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <Label>{setting.label}</Label>
              <p className="text-xs text-muted-foreground">{setting.description}</p>
            </div>
            <Switch
              checked={profile[key]}
              onCheckedChange={(checked) => onChange({ [key]: checked })}
            />
          </div>
        );
      })}
      {passthroughConfig?.supported && (
        <div className="flex items-center justify-between rounded-md border p-3">
          <div className="space-y-1">
            <Label>{passthroughConfig.label}</Label>
            <p className="text-xs text-muted-foreground">{passthroughConfig.description}</p>
          </div>
          <Switch
            checked={profile.cli_passthrough}
            onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
            disabled={lockPassthrough}
          />
        </div>
      )}
    </div>
  );
}

function capabilityStatusMessage(modelConfig: ModelConfig): string | null {
  switch (modelConfig.status) {
    case "probing":
      return "Checking agent capabilities…";
    case "auth_required":
      return "Authentication required. Run the agent CLI in your terminal to authenticate, then refresh.";
    case "not_installed":
      return "Agent CLI not installed.";
    case "failed":
      return `Probe failed${modelConfig.error ? `: ${modelConfig.error}` : ""}`;
    default:
      return null;
  }
}

function CapabilityStatusMessage({ modelConfig }: { modelConfig: ModelConfig }) {
  const msg = capabilityStatusMessage(modelConfig);
  if (!msg) return null;
  return (
    <p
      data-testid="profile-capability-status"
      data-status={modelConfig.status}
      className="text-xs text-muted-foreground"
    >
      {msg}
    </p>
  );
}

function ModelField({
  profile,
  modelConfig,
  onChange,
  agentName,
  isCompact,
}: {
  profile: ProfileFormData;
  modelConfig: ModelConfig;
  onChange: (patch: Partial<ProfileFormData>) => void;
  agentName: string;
  isCompact: boolean;
}) {
  return (
    <div className={isCompact ? "space-y-1.5" : "space-y-2"}>
      {isCompact ? (
        <Label className="text-xs text-muted-foreground">Model</Label>
      ) : (
        <Label>Model</Label>
      )}
      <ModelCombobox
        value={profile.model || modelConfig.default_model}
        onChange={(value) => onChange({ model: value })}
        models={modelConfig.available_models}
        defaultModel={modelConfig.default_model}
        placeholder="Select or enter model..."
        agentName={agentName}
        supportsDynamicModels={modelConfig.supports_dynamic_models}
      />
      <CapabilityStatusMessage modelConfig={modelConfig} />
    </div>
  );
}

function ModeField({
  profile,
  modelConfig,
  onChange,
  isCompact,
}: {
  profile: ProfileFormData;
  modelConfig: ModelConfig;
  onChange: (patch: Partial<ProfileFormData>) => void;
  isCompact: boolean;
}) {
  if (!modelConfig.available_modes || modelConfig.available_modes.length === 0) {
    return null;
  }
  const selected = profile.mode || modelConfig.current_mode_id || "";
  const activeMode = modelConfig.available_modes.find((m) => m.id === selected);
  return (
    <div data-testid="profile-mode-field" className={isCompact ? "space-y-1.5" : "space-y-2"}>
      {isCompact ? (
        <Label className="text-xs text-muted-foreground">Mode</Label>
      ) : (
        <Label>Mode</Label>
      )}
      <select
        data-testid="profile-mode-select"
        className="w-full rounded-md border bg-background px-3 py-2 text-sm cursor-pointer"
        value={selected}
        onChange={(event) => onChange({ mode: event.target.value })}
      >
        {modelConfig.available_modes.map((m) => (
          <option key={m.id} value={m.id}>
            {m.name}
          </option>
        ))}
      </select>
      {activeMode?.description && (
        <p className="text-xs text-muted-foreground">{activeMode.description}</p>
      )}
    </div>
  );
}

function NameField({
  profile,
  onChange,
  canRemove,
  onRemove,
}: {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  canRemove?: boolean;
  onRemove?: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="flex-1 space-y-2">
        <Label>Profile name</Label>
        <Input
          data-testid="profile-name-input"
          value={profile.name}
          onChange={(event) => onChange({ name: event.target.value })}
          placeholder="Default profile"
        />
      </div>
      {canRemove && onRemove && (
        <Button size="sm" variant="ghost" onClick={onRemove}>
          Remove
        </Button>
      )}
    </div>
  );
}

export function ProfileFormFields({
  profile,
  onChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  agentName,
  onRemove,
  canRemove = false,
  variant = "default",
  hideNameField = false,
  lockPassthrough = false,
}: ProfileFormFieldsProps) {
  const isCompact = variant === "compact";

  return (
    <div className={isCompact ? "space-y-3" : "space-y-4"}>
      {!hideNameField && (
        <NameField
          profile={profile}
          onChange={onChange}
          canRemove={canRemove}
          onRemove={onRemove}
        />
      )}

      <ModelField
        profile={profile}
        modelConfig={modelConfig}
        onChange={onChange}
        agentName={agentName}
        isCompact={isCompact}
      />

      <ModeField
        profile={profile}
        modelConfig={modelConfig}
        onChange={onChange}
        isCompact={isCompact}
      />

      <PermissionToggles
        profile={profile}
        onChange={onChange}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
        variant={variant}
        lockPassthrough={lockPassthrough}
      />
    </div>
  );
}
