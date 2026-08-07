"use client";

import { useState } from "react";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Tier } from "@/lib/state/slices/office/types";
import {
  CreateProfileButton,
  CreateProfilePanel,
  fillMissingTierProfileIds,
  useSelectableProfileOptions,
} from "./agent-profile-setup-controls";
import { useTranslation } from "react-i18next";

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

// Catalog keys, not copy: this is module scope, so a `t()` here would resolve
// once at import and freeze at the boot locale. The record keys are the wire
// `Tier` values and stay untranslated.
//
// `usageLabel` is a whole aria-label per tier rather than a "{{label}} tier
// usage" frame with the tier name interpolated: the name is itself translated,
// and a translated value inside a translated frame can neither reorder nor
// agree.
const TIER_PROFILE_COPY: Record<Tier, { label: string; description: string; usageLabel: string }> =
  {
    frontier: {
      label: "office:tierFrontier",
      description: "office:tierFrontierDescription",
      usageLabel: "office:tierFrontierUsage",
    },
    balanced: {
      label: "office:tierBalanced",
      description: "office:tierBalancedDescription",
      usageLabel: "office:tierBalancedUsage",
    },
    economy: {
      label: "office:tierEconomy",
      description: "office:tierEconomyDescription",
      usageLabel: "office:tierEconomyUsage",
    },
  };

export function StepTierProfiles({
  tierProfileIds,
  agentProfiles,
  onChange,
  onAgentProfilesChange,
}: StepTierProfilesProps) {
  const { t } = useTranslation();
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const setAgentProfiles = useAppStore((s) => s.setAgentProfiles);
  const { profileOptions } = useSelectableProfileOptions(agentProfiles);
  const [showCreate, setShowCreate] = useState(profileOptions.length === 0);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{t("office:setupTierAgentProfiles")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:chooseTheProfileFamilyOfficeShould")}
        </p>
      </div>
      <div className="space-y-4">
        <div className="space-y-3">
          <div>
            <Label>{t("office:agentTierProfiles")}</Label>
            <p className="text-xs text-muted-foreground mt-1">
              {t("office:theCoordinatorCanCreateWorkerAgents")}
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
            setAgentProfiles={setAgentProfiles}
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
  const { t } = useTranslation();
  const copy = TIER_PROFILE_COPY[tier];
  return (
    <div className="min-w-0 space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium">{t(copy.label)}</Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex size-4 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={t(copy.usageLabel)}
            >
              <IconInfoCircle className="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs" side="top">
            {t(copy.description)}
          </TooltipContent>
        </Tooltip>
      </div>
      <AgentSelector
        options={options}
        value={value}
        onValueChange={onChange}
        disabled={options.length === 0}
        placeholder={t("office:selectProfile")}
        triggerClassName="border border-input rounded-md px-3 h-9 w-full"
      />
    </div>
  );
}
