"use client";

import { useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { getExecutorIcon } from "@/lib/executor-icons";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { Tier } from "@/lib/state/slices/office/types";
import { seedTier } from "./seed-tier-mapping";
import {
  CreateProfileButton,
  CreateProfilePanel,
  useSelectableProfileOptions,
} from "./agent-profile-setup-controls";
import { Trans, useTranslation } from "react-i18next";
import { TIER_NAME_KEYS } from "../lib/label-keys";

type ProfileSelectOption = ReturnType<typeof useSelectableProfileOptions>["profileOptions"][number];

type StepAgentProps = {
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  defaultTier?: Tier;
  agentProfiles: AgentProfileOption[];
  onChange: (patch: {
    agentName?: string;
    agentProfileId?: string;
    executorPreference?: string;
    defaultTier?: Tier;
  }) => void;
  onAgentProfilesChange?: (profiles: AgentProfileOption[]) => void;
};

// Fallback used only when meta has not been hydrated yet (graceful degradation).
// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// `id`s are the persisted executor-type values.
const FALLBACK_EXECUTOR_OPTIONS = [
  {
    id: "local_pc",
    labelKey: "office:localStandalone",
    descriptionKey: "office:runOnHostMachine",
  },
  {
    id: "local_docker",
    labelKey: "office:localDocker",
    descriptionKey: "office:runInALocalDockerContainer",
  },
  {
    id: "sprites",
    labelKey: "office:spritesRemoteSandbox",
    descriptionKey: "office:runInASpritesCloudEnvironment",
  },
];

export function StepAgent({
  agentName,
  agentProfileId,
  executorPreference,
  defaultTier,
  agentProfiles,
  onChange,
  onAgentProfilesChange,
}: StepAgentProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const executorOptions =
    meta?.executorTypes ??
    FALLBACK_EXECUTOR_OPTIONS.map((o) => ({
      id: o.id,
      label: t(o.labelKey),
      description: t(o.descriptionKey),
    }));
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const setAgentProfiles = useAppStore((s) => s.setAgentProfiles);

  const { sortedProfiles, profileOptions } = useSelectableProfileOptions(agentProfiles);

  const selectedProfile = sortedProfiles.find((p) => p.id === agentProfileId);
  const [showCreate, setShowCreate] = useState(profileOptions.length === 0);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{t("office:createYourCoordinatorAgent")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:theCoordinatorManagesOtherAgentsDelegates")}
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="agent-name">{t("office:agentName")}</Label>
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
            selectedProfile={selectedProfile}
            onChange={onChange}
            onCreateClick={() => setShowCreate(true)}
          />
          {showCreate && (
            <CreateProfilePanel
              settingsAgents={settingsAgents}
              wizardProfiles={agentProfiles}
              canCancel={profileOptions.length > 0}
              setAgentProfiles={setAgentProfiles}
              onAgentProfilesChange={onAgentProfilesChange}
              onProfileSaved={(profileId) => onChange({ agentProfileId: profileId })}
              onClose={() => setShowCreate(false)}
            />
          )}
        </div>
        <ExecutorSelector
          value={executorPreference}
          options={executorOptions}
          onChange={(v) => onChange({ executorPreference: v })}
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
  selectedProfile,
  onChange,
  onCreateClick,
}: {
  showCreate: boolean;
  profileOptions: ProfileSelectOption[];
  agentProfileId: string;
  selectedProfile: AgentProfileOption | undefined;
  onChange: StepAgentProps["onChange"];
  onCreateClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Label>{t("office:cliAgentProfile")}</Label>
      {!showCreate && (
        <AgentSelector
          options={profileOptions}
          value={agentProfileId}
          onValueChange={(v) => onChange({ agentProfileId: v })}
          disabled={profileOptions.length === 0}
          placeholder={t("office:selectAnAgentProfile")}
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

function TierIndicator({
  selectedProfile,
  defaultTier,
  onChange,
}: {
  selectedProfile: AgentProfileOption | undefined;
  defaultTier?: Tier;
  onChange: (t: Tier) => void;
}) {
  const { t } = useTranslation();
  // The label string in AgentProfileOption is "<agent display> • <profile name>"
  // — fall back to the raw label when we cannot extract a model id, since the
  // seed mapping only matters for the "we'll treat X as the Y tier" hint.
  const modelHint = selectedProfile?.label;
  const seeded = seedTier(selectedProfile?.agent_id, modelHint);
  const value: Tier = defaultTier ?? seeded;
  return (
    <div>
      <Label>{t("office:workspaceDefaultTier")}</Label>
      {/*
        One sentence, one key. Splitting it into "We'll treat" + a model chip +
        " as the " + a tier id + " tier for " freezes English word order, and the
        tier arrived as a raw wire value (`frontier`) doubling as display copy.
        The tier now travels as a translated name resolved from TIER_NAME_KEYS.
      */}
      <p className="text-xs text-muted-foreground mb-2">
        <Trans
          i18nKey="office:workspaceDefaultTierHint"
          values={{
            model: modelHint || t("office:yourModel"),
            tier: t(TIER_NAME_KEYS[value]),
            provider: selectedProfile?.agent_name || t("office:thisProvider"),
          }}
        >
          We&apos;ll treat <span className="font-mono">model</span> as the tier for the provider.
          Change it later in Workspace → Provider routing.
        </Trans>
      </p>
      <ToggleGroup
        type="single"
        value={value}
        onValueChange={(v) => v && onChange(v as Tier)}
        className="justify-start"
      >
        <ToggleGroupItem value="frontier" className="cursor-pointer capitalize">
          {t("office:tierFrontier")}
        </ToggleGroupItem>
        <ToggleGroupItem value="balanced" className="cursor-pointer capitalize">
          {t("office:tierBalanced")}
        </ToggleGroupItem>
        <ToggleGroupItem value="economy" className="cursor-pointer capitalize">
          {t("office:tierEconomy")}
        </ToggleGroupItem>
      </ToggleGroup>
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
  const { t } = useTranslation();
  if (!hasProfiles) {
    return (
      <div className="mt-2 text-xs text-muted-foreground space-y-1">
        <p>{t("office:noCliAgentProfilesAvailableYet")}</p>
        <CreateProfileButton hasProfiles={false} onCreateClick={onCreateClick} />
      </div>
    );
  }
  return (
    <div className="mt-2 space-y-2 text-xs text-muted-foreground">
      {selected ? (
        <div className="flex items-center gap-2 flex-wrap">
          <Badge variant="secondary">{selected.agent_name}</Badge>
          {selected.cli_passthrough ? (
            <Badge variant="outline">{t("common:cliPassthrough")}</Badge>
          ) : null}
        </div>
      ) : (
        <p>{t("office:picksTheCliClientModelMode")}</p>
      )}
      <CreateProfileButton hasProfiles={hasProfiles} onCreateClick={onCreateClick} />
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
  const { t } = useTranslation();
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
      disabledReason: disabled ? t("office:comingSoonOnlyLocalIsSupported") : undefined,
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
      <Label>{t("office:executorPreference")}</Label>
      <Combobox
        options={comboOptions}
        value={current}
        onValueChange={onChange}
        placeholder={t("office:selectExecutor")}
        showSearch={false}
        triggerClassName="mt-1 border border-input rounded-md px-3 h-9"
      />
      {selected ? (
        <p className="text-xs text-muted-foreground mt-1">{selected.description}</p>
      ) : null}
    </div>
  );
}
