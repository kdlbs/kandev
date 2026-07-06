"use client";

import { useMemo, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { getCapabilityWarning } from "@/lib/capability-warning";
import { CliProfileEditor } from "@/components/agent/cli-profile-editor";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { getExecutorIcon } from "@/lib/executor-icons";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { Tier } from "@/lib/state/slices/office/types";
import { seedTier } from "./seed-tier-mapping";

type StepAgentProps = {
  agentName: string;
  agentProfileId: string;
  tierProfileIds: Partial<Record<Tier, string>>;
  executorPreference: string;
  defaultTier?: Tier;
  agentProfiles: AgentProfileOption[];
  onChange: (patch: {
    agentName?: string;
    agentProfileId?: string;
    tierProfileIds?: Partial<Record<Tier, string>>;
    executorPreference?: string;
    defaultTier?: Tier;
  }) => void;
  onAgentProfilesChange?: (profiles: AgentProfileOption[]) => void;
};

// Fallback used only when meta has not been hydrated yet (graceful degradation).
const FALLBACK_EXECUTOR_OPTIONS = [
  { id: "local_pc", label: "Local (standalone)", description: "Run on host machine" },
  { id: "local_docker", label: "Local Docker", description: "Run in a local Docker container" },
  {
    id: "sprites",
    label: "Sprites (remote sandbox)",
    description: "Run in a Sprites cloud environment",
  },
];

function sortProfiles(profiles: AgentProfileOption[]): AgentProfileOption[] {
  return [...profiles].sort((a, b) => {
    const aDisabled =
      a.cli_passthrough || !!getCapabilityWarning(a.capability_status, a.capability_error);
    const bDisabled =
      b.cli_passthrough || !!getCapabilityWarning(b.capability_status, b.capability_error);
    if (aDisabled === bDisabled) return 0;
    return aDisabled ? 1 : -1;
  });
}

export function StepAgent({
  agentName,
  agentProfileId,
  tierProfileIds,
  executorPreference,
  defaultTier,
  agentProfiles,
  onChange,
  onAgentProfilesChange,
}: StepAgentProps) {
  const meta = useAppStore((s) => s.office.meta);
  const executorOptions = meta?.executorTypes ?? FALLBACK_EXECUTOR_OPTIONS;
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const setAgentProfiles = useAppStore((s) => s.setAgentProfiles);
  const agentProfilesState = useAppStore((s) => s.agentProfiles.items);

  const sortedProfiles = useMemo(() => sortProfiles(agentProfiles), [agentProfiles]);
  const baseOptions = useAgentProfileOptions(sortedProfiles);
  const profileOptions = useMemo(
    () =>
      baseOptions.map((opt, i) => ({
        ...opt,
        disabled:
          sortedProfiles[i]?.cli_passthrough ||
          !!getCapabilityWarning(
            sortedProfiles[i]?.capability_status,
            sortedProfiles[i]?.capability_error,
          ),
      })),
    [baseOptions, sortedProfiles],
  );

  const selectedProfile = sortedProfiles.find((p) => p.id === agentProfileId);
  const [showCreate, setShowCreate] = useState(profileOptions.length === 0);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Create your coordinator agent</h2>
        <p className="text-sm text-muted-foreground mt-1">
          The coordinator manages other agents, delegates tasks, and monitors progress.
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="agent-name">Agent name</Label>
          <Input
            id="agent-name"
            value={agentName}
            onChange={(e) => onChange({ agentName: e.target.value })}
            placeholder="CEO"
            className="mt-1"
            autoFocus
          />
        </div>
        <div>
          <ProfileSelectorSection
            showCreate={showCreate}
            profileOptions={profileOptions}
            agentProfileId={agentProfileId}
            tierProfileIds={tierProfileIds}
            selectedProfile={selectedProfile}
            onChange={onChange}
            onCreateClick={() => setShowCreate(true)}
          />
          {showCreate && (
            <CreateProfilePanel
              settingsAgents={settingsAgents}
              storeProfiles={agentProfilesState}
              wizardProfiles={agentProfiles}
              canCancel={profileOptions.length > 0}
              setAgentProfiles={setAgentProfiles}
              onAgentProfilesChange={onAgentProfilesChange}
              onChange={onChange}
              onClose={() => setShowCreate(false)}
            />
          )}
        </div>
        <ExecutorSelector
          value={executorPreference}
          options={executorOptions}
          onChange={(v) => onChange({ executorPreference: v })}
        />
        <TierProfileSelectorGroup
          tierProfileIds={tierProfileIds}
          profileOptions={profileOptions}
          onChange={(tier, profileId) =>
            onChange({ tierProfileIds: { ...tierProfileIds, [tier]: profileId } })
          }
        />
        <TierIndicator
          selectedProfile={selectedProfile}
          defaultTier={defaultTier}
          onChange={(t) => onChange({ defaultTier: t })}
        />
      </div>
    </div>
  );
}

function ProfileSelectorSection({
  showCreate,
  profileOptions,
  agentProfileId,
  tierProfileIds,
  selectedProfile,
  onChange,
  onCreateClick,
}: {
  showCreate: boolean;
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  agentProfileId: string;
  tierProfileIds: Partial<Record<Tier, string>>;
  selectedProfile: AgentProfileOption | undefined;
  onChange: StepAgentProps["onChange"];
  onCreateClick: () => void;
}) {
  return (
    <>
      <Label>CLI agent profile</Label>
      {!showCreate && (
        <AgentSelector
          options={profileOptions}
          value={agentProfileId}
          onValueChange={(v) =>
            onChange({
              agentProfileId: v,
              tierProfileIds: fillMissingTierProfileIds(tierProfileIds, v),
            })
          }
          disabled={profileOptions.length === 0}
          placeholder="Select an agent profile..."
          triggerClassName="mt-1 border border-input rounded-md px-3 h-9"
        />
      )}
      {!showCreate && (
        <ProfilePickerHint
          hasProfiles={profileOptions.length > 0}
          selected={selectedProfile}
          onCreateClick={onCreateClick}
        />
      )}
    </>
  );
}

function fillMissingTierProfileIds(
  current: Partial<Record<Tier, string>>,
  profileId: string,
): Partial<Record<Tier, string>> {
  return {
    frontier: current.frontier || profileId,
    balanced: current.balanced || profileId,
    economy: current.economy || profileId,
  };
}

const TIER_PROFILE_COPY: Record<Tier, { label: string; description: string }> = {
  frontier: {
    label: "Frontier",
    description:
      "Used when the coordinator creates agents for the highest-capability work or assigns the Frontier tier.",
  },
  balanced: {
    label: "Balanced",
    description: "Used for general worker agents when the coordinator assigns the Balanced tier.",
  },
  economy: {
    label: "Economy",
    description:
      "Used for QA, routine, and lower-cost agents when the coordinator assigns the Economy tier.",
  },
};

function TierProfileSelectorGroup({
  tierProfileIds,
  profileOptions,
  onChange,
}: {
  tierProfileIds: Partial<Record<Tier, string>>;
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  onChange: (tier: Tier, profileId: string) => void;
}) {
  return (
    <div className="space-y-3">
      <div>
        <Label>Agent tier profiles</Label>
        <p className="text-xs text-muted-foreground mt-1">
          These profiles become the workspace tier families used when the coordinator creates or
          schedules agents.
        </p>
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        {(["frontier", "balanced", "economy"] as const).map((tier) => (
          <TierProfileSelector
            key={tier}
            tier={tier}
            value={tierProfileIds[tier] ?? ""}
            options={profileOptions}
            onChange={(profileId) => onChange(tier, profileId)}
          />
        ))}
      </div>
    </div>
  );
}

function TierProfileSelector({
  tier,
  value,
  options,
  onChange,
}: {
  tier: Tier;
  value: string;
  options: ReturnType<typeof useAgentProfileOptions>;
  onChange: (profileId: string) => void;
}) {
  const copy = TIER_PROFILE_COPY[tier];
  return (
    <div className="min-w-0 space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium">{copy.label}</Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex size-4 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={`${copy.label} tier usage`}
            >
              <IconInfoCircle className="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs" side="top">
            {copy.description}
          </TooltipContent>
        </Tooltip>
      </div>
      <AgentSelector
        options={options}
        value={value}
        onValueChange={onChange}
        disabled={options.length === 0}
        placeholder="Select profile..."
        triggerClassName="border border-input rounded-md px-3 h-9 w-full"
      />
    </div>
  );
}

function TierIndicator({
  selectedProfile,
  defaultTier,
  onChange,
}: {
  selectedProfile: AgentProfileOption | undefined;
  defaultTier?: Tier;
  onChange: (t: Tier) => void;
}) {
  // The label string in AgentProfileOption is "<agent display> • <profile name>"
  // — fall back to the raw label when we cannot extract a model id, since the
  // seed mapping only matters for the "we'll treat X as the Y tier" hint.
  const modelHint = selectedProfile?.label;
  const seeded = seedTier(selectedProfile?.agent_id, modelHint);
  const value: Tier = defaultTier ?? seeded;
  return (
    <div>
      <Label>Workspace default tier</Label>
      <p className="text-xs text-muted-foreground mb-2">
        We&apos;ll treat <span className="font-mono">{modelHint || "your model"}</span> as the{" "}
        {value} tier for {selectedProfile?.agent_name || "this provider"}. Change it later in
        Workspace → Provider routing.
      </p>
      <ToggleGroup
        type="single"
        value={value}
        onValueChange={(v) => v && onChange(v as Tier)}
        className="justify-start"
      >
        <ToggleGroupItem value="frontier" className="cursor-pointer capitalize">
          Frontier
        </ToggleGroupItem>
        <ToggleGroupItem value="balanced" className="cursor-pointer capitalize">
          Balanced
        </ToggleGroupItem>
        <ToggleGroupItem value="economy" className="cursor-pointer capitalize">
          Economy
        </ToggleGroupItem>
      </ToggleGroup>
    </div>
  );
}

function CreateProfilePanel({
  settingsAgents,
  storeProfiles,
  wizardProfiles,
  canCancel,
  setAgentProfiles,
  onAgentProfilesChange,
  onChange,
  onClose,
}: {
  settingsAgents: { id: string; name: string }[];
  storeProfiles: AgentProfileOption[];
  wizardProfiles: AgentProfileOption[];
  canCancel: boolean;
  setAgentProfiles: (profiles: AgentProfileOption[]) => void;
  onAgentProfilesChange?: (profiles: AgentProfileOption[]) => void;
  onChange: StepAgentProps["onChange"];
  onClose: () => void;
}) {
  return (
    <div className="mt-2 rounded-md border bg-muted/30 p-3">
      <CliProfileEditor
        mode="create"
        defaultProfileName="default"
        showAdvanced
        allowCliPassthrough={false}
        onSaved={(saved) => {
          const agentForProfile = settingsAgents.find((a) => a.id === saved.agentId) ?? {
            id: saved.agentId ?? "",
            name: saved.agentId ?? "",
          };
          const option = toAgentProfileOption(agentForProfile, saved);
          setAgentProfiles([...storeProfiles.filter((p) => p.id !== option.id), option]);
          onAgentProfilesChange?.([...wizardProfiles.filter((p) => p.id !== option.id), option]);
          onChange({
            agentProfileId: saved.id,
            tierProfileIds: fillMissingTierProfileIds({}, saved.id),
          });
          onClose();
        }}
        onCancel={canCancel ? onClose : undefined}
      />
    </div>
  );
}

function ProfilePickerHint({
  hasProfiles,
  selected,
  onCreateClick,
}: {
  hasProfiles: boolean;
  selected: AgentProfileOption | undefined;
  onCreateClick: () => void;
}) {
  if (!hasProfiles) {
    return (
      <div className="mt-2 text-xs text-muted-foreground space-y-1">
        <p>No CLI agent profiles available yet.</p>
        <Button
          type="button"
          variant="link"
          onClick={onCreateClick}
          className="h-auto p-0 cursor-pointer text-primary"
        >
          Create one inline
        </Button>
      </div>
    );
  }
  return (
    <div className="mt-2 space-y-2 text-xs text-muted-foreground">
      {selected ? (
        <div className="flex items-center gap-2 flex-wrap">
          <Badge variant="secondary">{selected.agent_name}</Badge>
          {selected.cli_passthrough ? <Badge variant="outline">CLI passthrough</Badge> : null}
        </div>
      ) : (
        <p>Picks the CLI client, model, mode, and flags this agent will use.</p>
      )}
      <Button
        type="button"
        variant="link"
        onClick={onCreateClick}
        className="h-auto p-0 cursor-pointer text-primary"
      >
        + Create a new CLI profile
      </Button>
    </div>
  );
}

// Maps onboarding executor-preference IDs to the icon catalog keys in
// `lib/executor-icons.ts` (which uses runtime executor type names).
const EXECUTOR_ICON_TYPE: Record<string, string> = {
  local_pc: "local",
  local_docker: "local_docker",
  remote_docker: "remote_docker",
  sprites: "sprites",
};

function ExecutorSelector({
  value,
  options,
  onChange,
}: {
  value: string;
  options: { id: string; label: string; description: string }[];
  onChange: (v: string) => void;
}) {
  const current = value || "local_pc";
  const selected = options.find((o) => o.id === current);
  const comboOptions: ComboboxOption[] = options.map((opt) => {
    const Icon = getExecutorIcon(EXECUTOR_ICON_TYPE[opt.id] ?? "local");
    const disabled = opt.id !== "local_pc";
    return {
      value: opt.id,
      label: opt.label,
      description: opt.description,
      disabled,
      disabledReason: disabled ? "Coming soon — only Local is supported right now." : undefined,
      renderLabel: () => (
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">{opt.label}</span>
        </span>
      ),
    };
  });
  return (
    <div>
      <Label>Executor preference</Label>
      <Combobox
        options={comboOptions}
        value={current}
        onValueChange={onChange}
        placeholder="Select executor..."
        showSearch={false}
        triggerClassName="mt-1 border border-input rounded-md px-3 h-9"
      />
      {selected ? (
        <p className="text-xs text-muted-foreground mt-1">{selected.description}</p>
      ) : null}
    </div>
  );
}
