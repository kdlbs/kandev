"use client";

import {
  IconRefresh,
  IconAlertCircle,
  IconLock,
  IconPackageOff,
  IconTerminal2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ModeCombobox } from "@/components/settings/mode-combobox";
import { ModelCombobox } from "@/components/settings/model-combobox";
import { useAgentCapabilities } from "@/hooks/domains/settings/use-dynamic-models";
import { PERMISSION_KEYS, type PermissionKey } from "@/lib/agent-permissions";
import type {
  CommandEntry,
  ModelConfig,
  ModeEntry,
  ModelEntry,
  PermissionSetting,
  PassthroughConfig,
} from "@/lib/types/http";

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
      return "Probe failed. Check agent logs for details.";
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

function RefreshCapabilitiesButton({
  onRefresh,
  isLoading,
  error,
}: {
  onRefresh: () => Promise<void>;
  isLoading: boolean;
  error: string | null;
}) {
  return (
    <div className="flex items-center gap-2">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            onClick={onRefresh}
            disabled={isLoading}
            className="cursor-pointer"
            data-testid="profile-refresh-capabilities"
          >
            <IconRefresh className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>Refresh agent capabilities (models + modes)</p>
        </TooltipContent>
      </Tooltip>
      {error && (
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex items-center">
              <IconAlertCircle className="h-4 w-4 text-amber-500" />
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <p className="max-w-xs">Failed to refresh: {error}</p>
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function ModelPicker({
  profile,
  models,
  currentModelId,
  onChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  currentModelId: string | undefined;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  return (
    <ModelCombobox
      value={profile.model}
      onChange={(value) => onChange({ model: value })}
      models={models}
      currentModelId={currentModelId}
      placeholder="Select a model..."
    />
  );
}

function ModePicker({
  profile,
  modes,
  currentModeId,
  onChange,
}: {
  profile: ProfileFormData;
  modes: ModeEntry[];
  currentModeId: string | undefined;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  return (
    <ModeCombobox
      value={profile.mode}
      onChange={(value) => onChange({ mode: value })}
      modes={modes}
      currentModeId={currentModeId}
    />
  );
}

function CommandsButton({ commands }: { commands: CommandEntry[] }) {
  if (commands.length === 0) return null;
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          data-testid="profile-commands-button"
        >
          <IconTerminal2 className="mr-2 h-4 w-4" />
          Available commands ({commands.length})
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Available slash commands</DialogTitle>
          <DialogDescription>
            Type these during a session chat to invoke them — e.g. <code>/init</code>.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto space-y-2">
          {commands.map((c) => (
            <div key={c.name} className="rounded-md border p-3">
              <code className="text-sm font-semibold">/{c.name}</code>
              {c.description && (
                <p className="text-xs text-muted-foreground mt-1">{c.description}</p>
              )}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function NoAuthPanel({
  agentName,
  status,
  isLoading,
  onRefresh,
  error,
  rawError,
}: {
  agentName: string;
  status: "auth_required" | "not_installed";
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  rawError: string | null;
}) {
  const isAuth = status === "auth_required";
  const Icon = isAuth ? IconLock : IconPackageOff;
  const title = isAuth ? "No auth — login required" : "Not installed";
  const hint = isAuth ? (
    <>
      Run <code className="font-mono bg-muted px-1 py-0.5 rounded">{agentName} login</code> in your
      terminal, then click Refresh.
    </>
  ) : (
    <>Install the agent CLI, then click Refresh.</>
  );
  const detail = error || rawError;
  return (
    <div
      data-testid="profile-no-auth-panel"
      data-status={status}
      className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3"
    >
      <Icon className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
      <div className="flex-1 min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium">{title}</p>
          {detail && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="inline-flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-foreground hover:text-foreground cursor-help"
                  data-testid="profile-no-auth-details"
                >
                  <IconAlertCircle className="h-3 w-3" />
                  details
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-md">
                <p className="whitespace-pre-wrap break-words text-xs">{detail}</p>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={isLoading}
        className="cursor-pointer shrink-0"
        data-testid="profile-no-auth-refresh"
      >
        <IconRefresh className={`mr-2 h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
        Refresh
      </Button>
    </div>
  );
}

function CapabilitiesRow({
  profile,
  models,
  modes,
  commands,
  currentModelId,
  currentModeId,
  onChange,
  isCompact,
  isLoading,
  onRefresh,
  error,
  modelConfig,
  agentName,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  onChange: (patch: Partial<ProfileFormData>) => void;
  isCompact: boolean;
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  modelConfig: ModelConfig;
  agentName: string;
}) {
  const hasModes = modes.length > 0;
  const activeMode = hasModes
    ? modes.find((m) => m.id === (profile.mode || currentModeId || modes[0]?.id))
    : undefined;
  const labelCls = isCompact ? "text-xs text-muted-foreground" : undefined;
  const gapCls = isCompact ? "space-y-1.5" : "space-y-2";

  if (isLoading && models.length === 0) {
    return (
      <div className={gapCls}>
        <Label className={labelCls}>Start model</Label>
        <Skeleton className="h-9 w-full" />
      </div>
    );
  }

  const status = modelConfig.status;
  if (status === "auth_required" || status === "not_installed") {
    return (
      <NoAuthPanel
        agentName={agentName}
        status={status}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        rawError={modelConfig.error ?? null}
      />
    );
  }

  return (
    <div className={gapCls}>
      <div className="flex items-end gap-2">
        <div className={`flex-1 min-w-0 ${gapCls}`}>
          <Label className={labelCls}>Start model</Label>
          <ModelPicker
            profile={profile}
            models={models}
            currentModelId={currentModelId}
            onChange={onChange}
          />
        </div>
        {hasModes && (
          <div data-testid="profile-mode-field" className={`flex-1 min-w-0 ${gapCls}`}>
            <Label className={labelCls}>Start mode</Label>
            <ModePicker
              profile={profile}
              modes={modes}
              currentModeId={currentModeId}
              onChange={onChange}
            />
          </div>
        )}
        <RefreshCapabilitiesButton onRefresh={onRefresh} isLoading={isLoading} error={error} />
      </div>
      {activeMode?.description && (
        <p className="text-xs text-muted-foreground">{activeMode.description}</p>
      )}
      {commands.length > 0 && <CommandsButton commands={commands} />}
      <CapabilityStatusMessage modelConfig={modelConfig} />
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
        <Button size="sm" variant="ghost" className="cursor-pointer" onClick={onRemove}>
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
  const caps = useAgentCapabilities(agentName, modelConfig);

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

      <CapabilitiesRow
        profile={profile}
        models={caps.models}
        modes={caps.modes}
        commands={caps.commands}
        currentModelId={caps.currentModelId}
        currentModeId={caps.currentModeId}
        agentName={agentName}
        onChange={onChange}
        isCompact={isCompact}
        isLoading={caps.isLoading}
        onRefresh={caps.refresh}
        error={caps.error}
        modelConfig={modelConfig}
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
