'use client';

import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Switch } from '@kandev/ui/switch';
import { ModelCombobox } from '@/components/settings/model-combobox';
import { PERMISSION_KEYS, type PermissionKey } from '@/lib/agent-permissions';
import type { ModelConfig, PermissionSetting, PassthroughConfig } from '@/lib/types/http';

export type ProfileFormData = {
  name: string;
  model: string;
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
  variant?: 'default' | 'compact';
  hideNameField?: boolean;
  lockPassthrough?: boolean;
};

type PermissionToggleProps = {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  variant: 'default' | 'compact';
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
  const isCompact = variant === 'compact';
  const switchSize = isCompact ? ('sm' as const) : ('default' as const);

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
              <p className="text-xs text-muted-foreground">
                {setting.description}
              </p>
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
            <p className="text-xs text-muted-foreground">
              {passthroughConfig.description}
            </p>
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

export function ProfileFormFields({
  profile,
  onChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  agentName,
  onRemove,
  canRemove = false,
  variant = 'default',
  hideNameField = false,
  lockPassthrough = false,
}: ProfileFormFieldsProps) {
  const isCompact = variant === 'compact';

  return (
    <div className={isCompact ? 'space-y-3' : 'space-y-4'}>
      {!hideNameField && (
        <div className="flex items-center justify-between gap-4">
          <div className="flex-1 space-y-2">
            <Label>Profile name</Label>
            <Input
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
      )}

      <div className={isCompact ? 'space-y-1.5' : 'space-y-2'}>
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
      </div>

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
