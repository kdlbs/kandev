"use client";

import { useCallback, useMemo, useState } from "react";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { ModelCombobox } from "@/components/settings/model-combobox";
import { ModeCombobox } from "@/components/settings/mode-combobox";
import { CLIFlagsField } from "@/components/settings/cli-flags-field";
import {
  createAgentAction,
  createAgentProfileAction,
  updateAgentProfileAction,
} from "@/app/actions/agents";
import type { Agent, AgentProfile, AvailableAgent, CLIFlag } from "@/lib/types/http";
import { seedDefaultCLIFlags } from "@/lib/cli-flags";

export type CliProfileEditorMode = "create" | "edit";

export type CliProfileEditorProps = {
  /** "create" mounts the inline create flow; "edit" patches an existing profile. */
  mode: CliProfileEditorMode;
  /** When `mode === "edit"`, the existing profile to patch. */
  profile?: AgentProfile;
  /** Default profile name suggestion in create mode. */
  defaultProfileName?: string;
  /** Called once a profile is successfully saved. Receives the saved profile. */
  onSaved: (profile: AgentProfile) => void;
  /** Called on cancel; only used by callers that wrap us in a dialog. */
  onCancel?: () => void;
  /**
   * Show the advanced (permissions / passthrough) toggles. Defaults to
   * collapsed; the wizard hides them entirely, the configuration tab
   * exposes them inline.
   */
  showAdvanced?: boolean;
  /** Whether the editor exposes CLI passthrough. Office agents should keep ACP enabled. */
  allowCliPassthrough?: boolean;
};

type FormState = {
  agentName: string;
  profileName: string;
  model: string;
  mode: string;
  cliFlags: CLIFlag[];
  cliPassthrough: boolean;
  allowIndexing: boolean;
};

function pickDefaultAgent(available: AvailableAgent[]): AvailableAgent | undefined {
  return available.find((a) => a.available) ?? available[0];
}

function fromExistingProfile(profile: AgentProfile): FormState {
  return {
    agentName: profile.agentId ?? "",
    profileName: profile.name,
    model: profile.model ?? "",
    mode: profile.mode ?? "",
    cliFlags: profile.cliFlags ?? [],
    cliPassthrough: profile.cliPassthrough ?? false,
    allowIndexing: profile.allowIndexing ?? false,
  };
}

function fromDefaultAgent(
  defaultName: string,
  defaultAgent: AvailableAgent | undefined,
): FormState {
  const cfg = defaultAgent?.model_config;
  const allowIndex = defaultAgent?.permission_settings?.allow_indexing?.default ?? false;
  return {
    agentName: defaultAgent?.name ?? "",
    profileName: defaultName,
    model: cfg?.default_model ?? "",
    mode: cfg?.current_mode_id ?? "",
    cliFlags: seedDefaultCLIFlags(defaultAgent?.permission_settings ?? {}),
    cliPassthrough: false,
    allowIndexing: allowIndex,
  };
}

function initialState(
  mode: CliProfileEditorMode,
  profile: AgentProfile | undefined,
  defaultName: string,
  defaultAgent: AvailableAgent | undefined,
): FormState {
  if (mode === "edit" && profile) return fromExistingProfile(profile);
  return fromDefaultAgent(defaultName, defaultAgent);
}

export function CliProfileEditor({
  mode,
  profile,
  defaultProfileName = "default",
  onSaved,
  onCancel,
  showAdvanced = false,
  allowCliPassthrough = true,
}: CliProfileEditorProps) {
  const availableAgents = useAvailableAgents();
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const installed = useMemo(
    () => availableAgents.items.filter((a) => a.available),
    [availableAgents.items],
  );
  const [form, setForm] = useState<FormState>(() =>
    initialState(mode, profile, defaultProfileName, pickDefaultAgent(installed)),
  );
  const [saving, setSaving] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(showAdvanced);

  const currentAgent = installed.find((a) => a.name === form.agentName);
  const modelConfig = currentAgent?.model_config;
  const permissionSettings = currentAgent?.permission_settings;

  const patch = useCallback((p: Partial<FormState>) => {
    setForm((prev) => ({ ...prev, ...p }));
  }, []);

  const handleSave = useCallback(async () => {
    if (!form.agentName || !form.profileName.trim()) return;
    setSaving(true);
    try {
      const saved =
        mode === "edit" && profile
          ? await saveExistingProfile(profile.id, form)
          : await saveNewProfile(form, settingsAgents);
      onSaved(saved);
    } finally {
      setSaving(false);
    }
  }, [form, mode, profile, settingsAgents, onSaved]);

  const canSave = form.agentName !== "" && form.profileName.trim() !== "";

  return (
    <div className="space-y-4" data-testid="cli-profile-editor">
      {mode === "create" && (
        <CliClientPicker
          installed={installed}
          value={form.agentName}
          onChange={(name) =>
            patch({
              agentName: name,
              model: installed.find((a) => a.name === name)?.model_config.default_model ?? "",
              mode: installed.find((a) => a.name === name)?.model_config.current_mode_id ?? "",
              cliFlags: seedDefaultCLIFlags(
                installed.find((a) => a.name === name)?.permission_settings ?? {},
              ),
            })
          }
        />
      )}

      <div>
        <Label htmlFor="cli-profile-name">Profile name</Label>
        <Input
          id="cli-profile-name"
          value={form.profileName}
          onChange={(e) => patch({ profileName: e.target.value })}
          placeholder="default"
          className="mt-1"
        />
      </div>

      <ModelModeFields
        modelConfig={modelConfig ?? null}
        model={form.model}
        mode={form.mode}
        onModelChange={(v) => patch({ model: v })}
        onModeChange={(v) => patch({ mode: v })}
      />

      <AdvancedToggles
        open={advancedOpen}
        onToggle={() => setAdvancedOpen((o) => !o)}
        cliPassthrough={form.cliPassthrough}
        allowIndexing={form.allowIndexing}
        cliFlags={form.cliFlags}
        permissionSettings={permissionSettings ?? {}}
        allowCliPassthrough={allowCliPassthrough}
        showAllowIndexing={Boolean(permissionSettings?.allow_indexing?.supported)}
        onCliPassthroughChange={(v) => patch({ cliPassthrough: v })}
        onAllowIndexingChange={(v) => patch({ allowIndexing: v })}
        onCliFlagsChange={(v) => patch({ cliFlags: v })}
      />

      <EditorFooter
        mode={mode}
        saving={saving}
        canSave={canSave}
        onSave={handleSave}
        onCancel={onCancel}
      />
    </div>
  );
}

function buttonLabel(mode: CliProfileEditorMode, saving: boolean): string {
  if (saving) return mode === "edit" ? "Saving..." : "Creating...";
  return mode === "edit" ? "Save profile" : "Create profile";
}

function EditorFooter({
  mode,
  saving,
  canSave,
  onSave,
  onCancel,
}: {
  mode: CliProfileEditorMode;
  saving: boolean;
  canSave: boolean;
  onSave: () => void;
  onCancel?: () => void;
}) {
  return (
    <div className="flex items-center justify-end gap-2 pt-2">
      {onCancel && (
        <Button
          type="button"
          variant="ghost"
          onClick={onCancel}
          disabled={saving}
          className="cursor-pointer"
        >
          Cancel
        </Button>
      )}
      <Button
        type="button"
        onClick={onSave}
        disabled={!canSave || saving}
        className="cursor-pointer"
      >
        {buttonLabel(mode, saving)}
      </Button>
    </div>
  );
}

function CliClientPicker({
  installed,
  value,
  onChange,
}: {
  installed: AvailableAgent[];
  value: string;
  onChange: (name: string) => void;
}) {
  return (
    <div>
      <Label>CLI client</Label>
      <Select value={value} onValueChange={onChange} disabled={installed.length === 0}>
        <SelectTrigger className="mt-1 cursor-pointer">
          <SelectValue placeholder="Pick a CLI client" />
        </SelectTrigger>
        <SelectContent>
          {installed.map((agent) => (
            <SelectItem key={agent.name} value={agent.name} className="cursor-pointer">
              {agent.display_name || agent.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {installed.length === 0 && (
        <p className="text-xs text-muted-foreground mt-1">
          No CLI clients installed yet. Install Claude / Codex / OpenCode / Amp to enable this
          picker.
        </p>
      )}
    </div>
  );
}

type ModelModeFieldsProps = {
  modelConfig: NonNullable<AvailableAgent["model_config"]> | null;
  model: string;
  mode: string;
  onModelChange: (v: string) => void;
  onModeChange: (v: string) => void;
};

function ModelModeFields({
  modelConfig,
  model,
  mode,
  onModelChange,
  onModeChange,
}: ModelModeFieldsProps) {
  if (!modelConfig) {
    return (
      <p className="text-xs text-muted-foreground">
        Pick a CLI client to load its available models and modes.
      </p>
    );
  }
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <div>
        <Label>Model</Label>
        <ModelCombobox
          value={model}
          onChange={onModelChange}
          models={modelConfig.available_models ?? []}
          currentModelId={modelConfig.current_model_id}
        />
      </div>
      {(modelConfig.available_modes ?? []).length > 0 && (
        <div>
          <Label>Mode</Label>
          <ModeCombobox
            value={mode}
            onChange={onModeChange}
            modes={modelConfig.available_modes ?? []}
            currentModeId={modelConfig.current_mode_id}
          />
        </div>
      )}
    </div>
  );
}

type AdvancedTogglesProps = {
  open: boolean;
  onToggle: () => void;
  cliPassthrough: boolean;
  allowIndexing: boolean;
  cliFlags: CLIFlag[];
  permissionSettings: AvailableAgent["permission_settings"];
  allowCliPassthrough: boolean;
  showAllowIndexing: boolean;
  onCliPassthroughChange: (v: boolean) => void;
  onAllowIndexingChange: (v: boolean) => void;
  onCliFlagsChange: (v: CLIFlag[]) => void;
};

function AdvancedToggles({
  open,
  onToggle,
  cliPassthrough,
  allowIndexing,
  cliFlags,
  permissionSettings,
  allowCliPassthrough,
  showAllowIndexing,
  onCliPassthroughChange,
  onAllowIndexingChange,
  onCliFlagsChange,
}: AdvancedTogglesProps) {
  return (
    <div className="border-t pt-3">
      <button
        type="button"
        onClick={onToggle}
        className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
      >
        {open ? "Hide" : "Show"} advanced options
      </button>
      {open && (
        <div className="mt-3 space-y-3">
          {allowCliPassthrough && (
            <ToggleRow
              id="cli-passthrough"
              label="CLI passthrough"
              description="Forward stdin/stdout straight to the CLI subprocess. Disables ACP."
              checked={cliPassthrough}
              onChange={onCliPassthroughChange}
            />
          )}
          {showAllowIndexing && (
            <ToggleRow
              id="allow-indexing"
              label="Allow indexing"
              description="Permit the CLI to upload code for cloud indexing (auggie / similar)."
              checked={allowIndexing}
              onChange={onAllowIndexingChange}
            />
          )}
          <CLIFlagsField
            flags={cliFlags}
            permissionSettings={permissionSettings ?? {}}
            onChange={onCliFlagsChange}
            variant="compact"
          />
        </div>
      )}
    </div>
  );
}

function ToggleRow({
  id,
  label,
  description,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  description: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="space-y-0.5">
        <Label htmlFor={id} className="text-sm">
          {label}
        </Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} className="cursor-pointer" />
    </div>
  );
}

async function saveExistingProfile(id: string, form: FormState): Promise<AgentProfile> {
  return updateAgentProfileAction(id, {
    name: form.profileName.trim(),
    model: form.model,
    mode: form.mode || undefined,
    allow_indexing: form.allowIndexing,
    cli_flags: form.cliFlags,
    cli_passthrough: form.cliPassthrough,
  });
}

async function saveNewProfile(form: FormState, settingsAgents: Agent[]): Promise<AgentProfile> {
  const existingAgent = settingsAgents.find((a) => a.name === form.agentName);
  const profilePayload = {
    name: form.profileName.trim(),
    model: form.model,
    mode: form.mode || undefined,
    allow_indexing: form.allowIndexing,
    cli_passthrough: form.cliPassthrough,
    cli_flags: form.cliFlags,
  };
  if (existingAgent) {
    return createAgentProfileAction(existingAgent.id, profilePayload);
  }
  // Otherwise create the kanban Agent row + the first profile in one POST.
  const created = await createAgentAction({
    name: form.agentName,
    profiles: [profilePayload],
  });
  const newProfile = created.profiles.find((p) => p.name === profilePayload.name);
  if (!newProfile) {
    throw new Error(
      `Created agent ${form.agentName} but the response did not include the new profile.`,
    );
  }
  return newProfile;
}
