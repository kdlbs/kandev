'use client';

import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Switch } from '@kandev/ui/switch';
import { ModelCombobox } from '@/components/settings/model-combobox';
import type { ModelConfig, PermissionSetting, PassthroughConfig } from '@/lib/types/http';

export type ProfileFormData = {
  name: string;
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
  allow_indexing?: boolean;
  cli_passthrough: boolean;
};

export type ProfileFormFieldsProps = {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  agentName: string;
  onRemove?: () => void;
  canRemove?: boolean;
};

export function ProfileFormFields({
  profile,
  onChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  agentName,
  onRemove,
  canRemove = false,
}: ProfileFormFieldsProps) {
  return (
    <div className="space-y-4">
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

      <div className="space-y-2">
        <Label>Model</Label>
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

      <div className="grid gap-4 md:grid-cols-2">
        {permissionSettings.auto_approve?.supported && (
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <Label>{permissionSettings.auto_approve.label}</Label>
              <p className="text-xs text-muted-foreground">
                {permissionSettings.auto_approve.description}
              </p>
            </div>
            <Switch
              checked={profile.auto_approve}
              onCheckedChange={(checked) => onChange({ auto_approve: checked })}
            />
          </div>
        )}

        {permissionSettings.dangerously_skip_permissions?.supported && (
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <Label>{permissionSettings.dangerously_skip_permissions.label}</Label>
              <p className="text-xs text-muted-foreground">
                {permissionSettings.dangerously_skip_permissions.description}
              </p>
            </div>
            <Switch
              checked={profile.dangerously_skip_permissions}
              onCheckedChange={(checked) => onChange({ dangerously_skip_permissions: checked })}
            />
          </div>
        )}

        {permissionSettings.allow_indexing?.supported && (
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <Label>{permissionSettings.allow_indexing.label}</Label>
              <p className="text-xs text-muted-foreground">
                {permissionSettings.allow_indexing.description}
              </p>
            </div>
            <Switch
              checked={profile.allow_indexing ?? false}
              onCheckedChange={(checked) => onChange({ allow_indexing: checked })}
            />
          </div>
        )}

        {passthroughConfig?.supported && (
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Label>{passthroughConfig.label}</Label>
              </div>
              <p className="text-xs text-muted-foreground">
                {passthroughConfig.description}
              </p>
            </div>
            <Switch
              checked={profile.cli_passthrough}
              onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
            />
          </div>
        )}
      </div>
    </div>
  );
}
