"use client";

import { useState } from "react";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { useAgentsQuerySync } from "@/hooks/domains/settings/use-agents-query-sync";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Tier } from "@/lib/state/slices/office/types";
import {
  CreateProfileButton,
  CreateProfilePanel,
  fillMissingTierProfileIds,
  useSelectableProfileOptions,
} from "./agent-profile-setup-controls";

type StepTierProfilesProps = {
  tierProfileIds: Partial<Record<Tier, string>>;
  agentProfiles: AgentProfileOption[];
  onChange: (patch: {
    agentProfileId?: string;
    tierProfileIds?: Partial<Record<Tier, string>>;
  }) => void;
  onAgentProfilesChange?: (profiles: AgentProfileOption[]) => void;
};

const TIERS = ["frontier", "balanced", "economy"] as const;

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

export function StepTierProfiles({
  tierProfileIds,
  agentProfiles,
  onChange,
  onAgentProfilesChange,
}: StepTierProfilesProps) {
  const { settingsAgents, upsertProfile } = useAgentsQuerySync();
  const { profileOptions } = useSelectableProfileOptions(agentProfiles);
  const [showCreate, setShowCreate] = useState(profileOptions.length === 0);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Setup tier agent profiles</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Choose the profile family Office should use when agents are assigned Frontier, Balanced,
          or Economy work.
        </p>
      </div>
      <div className="space-y-4">
        <div className="space-y-3">
          <div>
            <Label>Agent tier profiles</Label>
            <p className="text-xs text-muted-foreground mt-1">
              The coordinator can create worker agents and choose a tier for each one. Each tier
              resolves to the profile selected here, and can be changed later in Workspace routing.
            </p>
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            {TIERS.map((tier) => (
              <TierProfileSelector
                key={tier}
                tier={tier}
                value={tierProfileIds[tier] ?? ""}
                options={profileOptions}
                onChange={(profileId) =>
                  onChange({ tierProfileIds: { ...tierProfileIds, [tier]: profileId } })
                }
              />
            ))}
          </div>
        </div>
        {!showCreate ? (
          <CreateProfileButton
            hasProfiles={profileOptions.length > 0}
            onCreateClick={() => setShowCreate(true)}
          />
        ) : null}
        {showCreate ? (
          <CreateProfilePanel
            settingsAgents={settingsAgents}
            wizardProfiles={agentProfiles}
            canCancel={profileOptions.length > 0}
            upsertProfile={upsertProfile}
            onAgentProfilesChange={onAgentProfilesChange}
            onProfileSaved={(profileId) =>
              onChange({
                agentProfileId: profileId,
                tierProfileIds: fillMissingTierProfileIds(tierProfileIds, profileId),
              })
            }
            onClose={() => setShowCreate(false)}
          />
        ) : null}
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
  options: ReturnType<typeof useSelectableProfileOptions>["profileOptions"];
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
